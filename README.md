# InPlaceUpdate
support k8s pod in place update with original deployment  controlled. Conatins a controller &amp; a tool  inplaceUpdate to update pod of a deployment

## 1.controller & tool
### controller - the main
The controller watch the pod with annotation key InPlaceUpdate:True, set pod.Status.Conditions type InPlaceUpdate to be True.

### tool - cmd/tool/inplaceUpdate
A tool to help update pod in place without recreate.

## 2.example
### a.prepare two images make sure the imageId is different.
### b.run controller.
### c.add annotaion inplaceUpdate and readinessGates to deployment
  ```
  template:
    metadata:
      annotations:
        InPlaceUpdate: "True"
      labels:
        k8s-app: busybox-test
    spec:
      readinessGates:
      - conditionType: InPlaceUpdate
      ...
```

### c.update pod with inplaceUpdate
```
InPlaceUpdate$ ./cmd/tool/inplaceUpdate -h
tools to update image of a deployment with InPlaceUpdate.

Usage:
  podIPU [flags]

Flags:
  -t, --GracePeriodSeconds duration   GracePeriodSeconds before really update image of container (default 5s)
  -d, --deployment string             deployment name
  -h, --help                          help for podIPU
  -i, --image string                  image to be updated
  -c, --kubeconfig string             (optional) absolute path to the kubeconfig file (default "/home/caden/.kube/config")
  -n, --namespace string              namespace of deployment (default "default")
  -v, --version                       Print version information and quit

./cmd/tool/inplaceUpdate -d busybox-test -i busybox:1.28.0-glibc-test -t 6s
```
after update the deployment will add a annotation with InPlaceUpdate:busybox:1.28.0-glibc-test(The latest image name updated). That means each pod of this deployment has been updated with new image.

***Note: We can only use tool to update deployment in placed, if we change image of deployment, pods will be recreated***