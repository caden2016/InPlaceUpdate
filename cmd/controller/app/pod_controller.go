package app

import (
	cmdutil "InPlaceUpdate/cmd"
	"InPlaceUpdate/cmd/controller/options"
	podcontroller "InPlaceUpdate/pkg/controller/pod"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"
	"os"
	"path/filepath"
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
		Use:  "podcontroller",
		Long: `PodController for support k8s Pod InPlaceUpdate.`,
		Run: func(cmd *cobra.Command, args []string) {
			if opt.Version {
				printVersion()
			}
			var stopCh = make(chan struct{})
			klog.Info(opt.KubeConfig)
			go run(opt.KubeConfig ,stopCh)
			cmdutil.Wait(func() { klog.Info("exiting.pod controller") }, stopCh)
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
	opt.KubeConfig = "./config"
	return cmd
}

func printVersion() {
	fmt.Printf("kubeutil version: %s\n", Version)
	os.Exit(0)
}

func run(kubeconfig string, stopChan <-chan struct{}) {
	klog.Info("Start PodController for support InPlaceUpdate")
	klog.Infof("kubeconfig path:%s",kubeconfig)
	pc,err := podcontroller.NewPodController(kubeconfig,stopChan)
	if err != nil {
		klog.Fatal(err)
	}
	klog.Info("start controller")
	pc.Start(1)
}