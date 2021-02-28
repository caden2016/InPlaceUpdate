package pod

import (
	"context"
	"fmt"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"strings"
	"time"
)
const (
	InPlaceUpdateAnnotation = "InPlaceUpdate"
)

type PodController struct  {
	name string
	queue workqueue.RateLimitingInterface
	podLister listerv1.PodLister
	podInformer cache.SharedIndexInformer
	clientset *kubernetes.Clientset
	stopChan <-chan struct{}
}

func NewPodController(kubeconfig_path string, stopChan <-chan struct{}) (pc *PodController, err error){
	RestConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig_path)
	if err != nil {
		return nil,err
	}
	//RestConfig, _ := tool.LoadKubeConfig("./config")
	ClientSet, err := kubernetes.NewForConfig(RestConfig)
	if err!=nil {
		return nil,err
	}
	sharedInformerFactory := informers.NewSharedInformerFactory(ClientSet,0)
	podInformer := coreinformers.New(sharedInformerFactory, v1.NamespaceAll, nil).Pods()
	podcontroller := &PodController{
		name: "PodController",
		podLister: podInformer.Lister(),
		podInformer:podInformer.Informer(),
		queue:workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pod-controller"),
		clientset:ClientSet,
		stopChan: stopChan,
	}

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			podcontroller.enqueuePod(obj)
			//klog.Infof("Pod %s added", pod.Name)

		},
		UpdateFunc: func(old, cur interface{}) {
			oldPod := old.(*v1.Pod)
			newPod := cur.(*v1.Pod)
			//klog.Infof("Pod %s added", pod.Name)
			if oldPod.ResourceVersion == newPod.ResourceVersion {
				klog.Infof("pod with the same ResourceVersion:%s",newPod.ResourceVersion)
				return
			}
			podcontroller.enqueuePod(cur)
		},
	})

	go sharedInformerFactory.Start(stopChan)
	return podcontroller, nil
}

func (dc *PodController) enqueuePod(obj interface{}) {
	pod := obj.(*v1.Pod)
	key,err := cache.DeletionHandlingMetaNamespaceKeyFunc(pod)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Couldn't get key for object %#v: %v", obj, err))
	}
	dc.queue.Add(key)
}

func (dc *PodController) processNextWorkItem() bool {
	key, quit := dc.queue.Get()
	if quit {
		return false
	}
	defer dc.queue.Done(key)
	err := dc.defaultHandleFunc(key)
	if err == nil {
		dc.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("handle %q failed with %v", key, err))
	dc.queue.AddRateLimited(key)
	return true
}

func (dc *PodController) defaultHandleFunc(key interface{}) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key.(string))
	klog.Infof("%s:%s",namespace,name)
	if err != nil {
		return err
	}
	pod, err := dc.podLister.Pods(namespace).Get(name)
	if err != nil {
		klog.Errorf("[%s:%s] podLister err:%v",namespace,name,err)
		return err
	}
	if _,ok := pod.Annotations[InPlaceUpdateAnnotation]; ok {
		for _,podcon := range pod.Status.Conditions {
			if podcon.Type == InPlaceUpdateAnnotation {
				return  nil
			}
		}
		podcondition := v1.PodCondition{
			Type:InPlaceUpdateAnnotation,
			Status:"True",
			LastProbeTime:metav1.Time{time.Now()},
			LastTransitionTime:metav1.Time{time.Now()},
		}
		pod.Status.Conditions = append(pod.Status.Conditions,podcondition)
		klog.Infof("[%s:%s] podLister podcondition:%v",namespace,name,podcondition)
		_,err :=dc.clientset.CoreV1().Pods(namespace).UpdateStatus(context.TODO(),pod,metav1.UpdateOptions{})
		if err!= nil {
			klog.Errorf("[%s:%s] podUpdate err:%v",namespace,name,err)
		}
		//newpodb,_ := json.Marshal(newpod)
		//oldpodb,_ := json.Marshal(pod)
		//klog.Info(string(oldpodb))
		//klog.Info(string(newpodb))
	}
	return nil
}

func (dc *PodController) worker() {
	for dc.processNextWorkItem() {
	}
}

func (dc *PodController) Start(workers int){
	defer utilruntime.HandleCrash()
	defer dc.queue.ShutDown()
	controllerName := strings.ToLower(dc.name)
	klog.Infof("Starting %v controller", controllerName)
	defer klog.Infof("Shutting down %s controller", controllerName)

	if !cache.WaitForNamedCacheSync(dc.name, dc.stopChan, dc.podInformer.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(dc.worker, time.Second, dc.stopChan)
	}
	<-dc.stopChan
}