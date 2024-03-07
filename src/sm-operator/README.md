# sm-operator

The sm-operator project is a Kubernetes operator designed to synchronize Bitwarden Secrets Manager secrets into K8s secrets.

## Description

The sm-operator uses a [controller](internal/controller/bitwardensecret_controller.go) to synchronize Bitwarden Secrets into Kubernetes secrets.  It does so by registering a Custom Resource Definition of BitwardenSecret into the cluster.  It will listen for new BitwardenSecrets registered on the cluster and then synchronize on a configurable interval.

## Getting Started

You willll need a Kubernetes cluster to run against. We recommend [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.

**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.  The controller ([internal/controller/bitwardensecret_controller.go](internal/controller/bitwardensecret_controller.go)) is where the main synchronization/reconciliation takes place.  The types file ([api/v1/bitwardensecret_types.go](api/v1/bitwardensecret_types.go)) specifies the structure of the Custom Resource Definition used throughout the controller, as well as the manifest structure.

The [config](config/) directory contains the generated manifest definitions for deployment and testing of the operator into kubernetes.

#### Modifying the API definitions

If you are editing the API definitions via [api/v1/bitwardensecret_types.go](api/v1/bitwardensecret_types.go), re-generate the manifests such as the custom resource definition using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

### Debugging

1. Install the Custom Resource Definition into the cluster using `make install` or by using the Visual Studio Task called "apply-crd" from the "Tasks: Run Task" in the command palette.

1. To debug the code, just hit F5.  You can also use `make run` at the command line to run without debugging.

**NOTE:** You can also run this in one step by running: `make install run`

### Uninstall Custom Resource Definition

To delete the CRDs from the cluster:

```sh
make uninstall
```

### Running on Kind cluster

1. Build and push your image directly to Kind by using the Visual Studio Code Command Palette.  Open the palette and select Tasks: Run Task and select "kind-push".

1. Deploy the Kubernetes objects to Kind by using the Visual Studio Code Command Palette.  Open the palette and select Tasks: Run Task and select "kind-deploy".

1. Create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object: `kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"`

1. Install an instances of BitwardenSecret.  An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml):  `kubectl apply -f -n some-namespace config/samples/k8s_v1_bitwardensecret.yaml`

### Alternative: Running on a cluster

1. Build and push your image to the registry location specified by `IMG`: `make docker-build docker-push IMG=<some-registry>/sm-operator:tag`

1. Deploy the controller to the cluster with the image specified by `IMG`: `make deploy IMG=<some-registry>/sm-operator:tag`

1. Create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object: `kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"`

1. Install an instances of BitwardenSecret.  An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml):  `kubectl apply -f -n some-namespace config/samples/k8s_v1_bitwardensecret.yaml`

### Undeploy controller

To "UnDeploy" the controller from the cluster after testing, run:

```sh
make undeploy
```

## Contributing

This project is open to public contribution, but you must follow the [Bitwarden Contribution Guidelines](https://contributing.bitwarden.com/).

## License

Source code in this repository is covered by one of two licenses: (i) the
GNU General Public License (GPL) v3.0 (ii) the Bitwarden License v1.0. The
default license throughout the repository is GPL v3.0 unless the header
specifies another license. Bitwarden Licensed code is found only in the
/bitwarden_license directory.

GPL v3.0:
https://github.com/bitwarden/server/blob/main/LICENSE_GPL.txt

Bitwarden License v1.0:
https://github.com/bitwarden/server/blob/main/LICENSE_BITWARDEN.txt

No grant of any rights in the trademarks, service marks, or logos of Bitwarden is
made (except as may be necessary to comply with the notice requirements as
applicable), and use of any Bitwarden trademarks must comply with Bitwarden
Trademark Guidelines
<https://github.com/bitwarden/server/blob/main/TRADEMARK_GUIDELINES.md>.
