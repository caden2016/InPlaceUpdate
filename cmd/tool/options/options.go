package options

import "time"

// KubeOptions is options for kubeutil.
type KubeOptions struct {
	Version bool
	KubeConfig string
	ImageName string
	DeploymentName string
	DeploymentNS string
	GracePeriodSeconds time.Duration
}

// NewkubeOptions creates a new KubeOptions with default config.
func NewkubeOptions() (*KubeOptions, error) {
	opt := KubeOptions{}
	return &opt, nil
}
