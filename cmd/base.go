package cmd

import (
	"k8s.io/klog"
	"os"
	"os/signal"
	"time"
)

func Die(stopCh chan struct{}) {
	time.Sleep(time.Second * 100)
	klog.Info("stop controller")
	close(stopCh)
}

func Wait(f func(), stopCh chan struct{}) {
	klog.Info("waiting..Interrupt or Kill to stop.")
	exit := make(chan os.Signal)
	// signal.Notify(exit, os.Kill, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGINT)
	signal.Notify(exit, os.Kill, os.Interrupt)
	for {
		select {
		case <-exit:
			close(stopCh)
			f()
			return
		}
	}
}
