package app

import (
	"InPlaceUpdate/cmd/tool/options"
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"
	"os"
	"path/filepath"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	appod "InPlaceUpdate/pkg/controller/pod"
	"time"
	"k8s.io/apimachinery/pkg/types"
)

var (
	// GitCommit git commit id
	GitCommit = "Unknown"
	// BuildTime build time
	BuildTime = "Unknown"
	// Version v1.0
	Version = "v1.0"
)

// NewKubeCommand creates a *cobra.Command object with default parameters
func NewKubeCommand() *cobra.Command {
	opt, err := options.NewkubeOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}
	var flags *pflag.FlagSet

	cmd := &cobra.Command{
		Use:  "podIPU",
		Long: `tools to update image of a deployment with InPlaceUpdate.`,
		Run: func(cmd *cobra.Command, args []string) {
			if opt.Version {
				printVersion()
			}
			var stopCh = make(chan struct{})
			klog.Info(opt.KubeConfig)
			run(opt,stopCh)
		},
	}
	flags = cmd.Flags()
	flags.BoolVarP(&opt.Version, "version", "v", false, "Print version information and quit")
	if home := homedir.HomeDir(); home != "" {
		flags.StringVarP(&opt.KubeConfig, "kubeconfig", "c", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flags.StringVarP(&opt.KubeConfig, "kubeconfig", "c", "./config", "(optional) absolute path to the kubeconfig file")
	}
	// flags.BoolVar(&opt.Version, "version", false, "Print version information and quit")
	flags.StringVarP(&opt.DeploymentNS, "namespace", "n", "default", "namespace of deployment")
	flags.StringVarP(&opt.DeploymentName, "deployment", "d", "", "deployment name")
	flags.StringVarP(&opt.ImageName, "image", "i", "", "image to be updated")
	flags.DurationVarP(&opt.GracePeriodSeconds, "GracePeriodSeconds", "t", 5*time.Second, "GracePeriodSeconds before really update image of container")
	return cmd
}

func printVersion() {
	fmt.Printf("kubeutil version: %s\n", Version)
	os.Exit(0)
}

func run(opt *options.KubeOptions, stopChan <-chan struct{}) {
	klog.Infof("update deploymeny:[%s:%s] with image:%s",opt.DeploymentNS,opt.DeploymentName,opt.ImageName)
	if opt.ImageName == "" {
		klog.Errorf("image cannot be empty")
		return
	}
	RestConfig, err := clientcmd.BuildConfigFromFlags("", opt.KubeConfig)
	if err != nil {
		klog.Fatal(err)
	}
	//RestConfig, _ := tool.LoadKubeConfig("./config")
	ClientSet, err := kubernetes.NewForConfig(RestConfig)
	if err!=nil {
		klog.Fatal(err)
	}

	deploy, err:=ClientSet.AppsV1().Deployments(opt.DeploymentNS).Get(context.TODO(),opt.DeploymentName,metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Errorf("deployment:[%s:%s] not found",opt.DeploymentNS,opt.DeploymentName)
		} else {
			klog.Errorf("Get deployment:[%s:%s] err:%v",opt.DeploymentNS,opt.DeploymentName,err)
		}
		return
	}

	//list pods created by deployment
	labelSelector := deploy.Spec.Selector
	labelMap,err := metav1.LabelSelectorAsMap(labelSelector)
	if err!=nil {
		klog.Errorf("deployment:[%s:%s] err:%v",opt.DeploymentNS,opt.DeploymentName,err)
		return
	}
	pods, err := ClientSet.CoreV1().Pods(opt.DeploymentNS).List(context.TODO(),metav1.ListOptions{LabelSelector: labels.SelectorFromSet(labelMap).String(),})
	if err!= nil {
		klog.Errorf("list pod in deployment:[%s:%s] err:%v",opt.DeploymentNS,opt.DeploymentName,err)
		return
	}

	//inPlaceUpdate each pod
	var success_count int = 0
	for _,v := range pods.Items {
		klog.Infof("update pod:[%s:%s] image:%s",v.Namespace,v.Name,opt.ImageName)
		if err = inPlaceUpdatePod(v.Namespace,v.Name,opt,ClientSet); err!=nil {
			klog.Errorf("update pod:[%s:%s] err:%v",v.Namespace,v.Name,err)
			success_count--
		}
		success_count++
	}

	if success_count !=len(pods.Items) {
		klog.Errorf("not all pod successfully updated, success_count:%d but pods:%d",success_count,len(pods.Items))
		return
	}

	//cannot update deployment image, will cause recreate pod
	//so just add annotation to notify the latest image updated
	klog.Infof("add annotation with %s:%s",appod.InPlaceUpdateAnnotation,opt.ImageName)
	deploy.ResourceVersion = ""
	deploy.Annotations[appod.InPlaceUpdateAnnotation] = opt.ImageName
	deployb,_:= json.Marshal(deploy)
	_, err =ClientSet.AppsV1().Deployments(opt.DeploymentNS).Patch(context.TODO(),opt.DeploymentName,types.MergePatchType,deployb,metav1.PatchOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Errorf("Patch deployment:[%s:%s] not found",opt.DeploymentNS,opt.DeploymentName)
		} else {
			klog.Errorf("Patch deployment:[%s:%s] err:%v",opt.DeploymentNS,opt.DeploymentName,err)
		}
		return
	}

}

func inPlaceUpdatePod(ns,name string,opt *options.KubeOptions,cs  *kubernetes.Clientset) error{
	pod, err:= cs.CoreV1().Pods(ns).Get(context.TODO(),name,metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("get pod:[%s:%s] not found",ns,name)
		} else {
			return fmt.Errorf("get pod:[%s:%s] err:%v",ns,name,err)
		}
	}

	var preImageId string
	for _,v :=  range pod.Status.ContainerStatuses {
		if v.Name == opt.DeploymentName {
			preImageId = v.ImageID
		}
	}

	//set pod to be unready, stop process traffic
	klog.Infof("set pod:[%s:%s] to be unready, stop process traffic",ns,name)
	for i,v := range pod.Status.Conditions {
		if v.Type == appod.InPlaceUpdateAnnotation {
			pod.Status.Conditions[i].Status = "False"
			pod.Status.Conditions[i].LastProbeTime = metav1.Time{time.Now()}
			pod.Status.Conditions[i].LastTransitionTime = metav1.Time{time.Now()}
		}
	}
	pod,err = cs.CoreV1().Pods(ns).UpdateStatus(context.TODO(),pod,metav1.UpdateOptions{})
	if err!= nil {
		return fmt.Errorf("stop process traffic, UpdateStatus pod:[%s:%s] err:%v",ns,name,err)
	}

	//set gracePeriodSeconds 5 sec
	klog.Infof("waiting %v before really update pod[%s:%s]",opt.GracePeriodSeconds,ns,name)
	time.Sleep(opt.GracePeriodSeconds)

	//update image
	for i,v := range pod.Spec.Containers {
		if v.Name == opt.DeploymentName {
			pod.Spec.Containers[i].Image = opt.ImageName
		}
	}

	pod.ResourceVersion = ""
	pod, err= cs.CoreV1().Pods(ns).Update(context.TODO(),pod,metav1.UpdateOptions{})
	if err!= nil {
		return fmt.Errorf("Update pod:[%s:%s] err:%v",ns,name,err)
	}

	klog.Infof("waiting for pod:[%s:%s] to be updated",ns,name)
	//get pod every 1 sec and judge if the imageId is changed, which measn container is updated
	stopChan := make(chan struct{})
	var isOK bool
	wait.Until(func(){
		pod, err= cs.CoreV1().Pods(ns).Get(context.TODO(),name,metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				klog.Errorf("get pod:[%s:%s] not found",ns,name)
			} else {
				klog.Errorf("get pod:[%s:%s] err:%v",ns,name,err)
			}
			isOK = false
			close(stopChan)
		}

		for _,v := range pod.Status.ContainerStatuses {
			if v.Name == opt.DeploymentName {
				if v.ImageID != preImageId {
					klog.Infof("pod:[%s:%s] ImageID changed",ns,name)
					isOK = true
					close(stopChan)
					break
				}
			}
		}

		klog.Info("1 second ...")
	}, time.Second, stopChan)

	if !isOK {
		return fmt.Errorf("get pod:[%s:%s] every 1 sec error",ns,name)
	}

	//set pod to be ready, start process traffic
	for i,v := range pod.Status.Conditions {
		if v.Type == appod.InPlaceUpdateAnnotation {
			pod.Status.Conditions[i].Status = "True"
			pod.Status.Conditions[i].LastProbeTime = metav1.Time{time.Now()}
			pod.Status.Conditions[i].LastTransitionTime = metav1.Time{time.Now()}
		}
	}
	pod,err = cs.CoreV1().Pods(ns).UpdateStatus(context.TODO(),pod,metav1.UpdateOptions{})
	if err!= nil {
		return fmt.Errorf("start process traffic, UpdateStatus pod:[%s:%s] err:%v",ns,name,err)
	}

	return nil
}