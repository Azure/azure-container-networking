# Kustomize based deployment

## Prerequisites

- [Kustomize](https://kustomize.io/) - Follow the instructions below to install it.

	```terminal
	curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash
	```

	For other installation options refer to https://kubectl.docs.kubernetes.io/installation/kustomize.

	To generate the resources for the **controller**, run the following command:

	```terminal
	kustomize build overlays/controller > /tmp/controller.yaml
	```

## Deploying to the cluster

### NPM Controller

To generate the resources for the **daemon**, run the following command:

```terminal
kustomize build overlays/daemon > /tmp/daemon.yaml
```

### NPM Daemon

> `kustomize` is not required for this step, since it is already bundled in the `kubectl` binary.

To deploy the daemon to your cluster, run the following command:
```terminal
kubectl apply -k overlays/daemon
```
