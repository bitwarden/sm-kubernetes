# sm-operator

The sm-operator project is a Kubernetes operator designed to synchronize Bitwarden Secrets Manager secrets into K8s secrets.

## Description

The sm-operator uses a [controller](internal/controller/bitwardensecret_controller.go) to synchronize Bitwarden Secrets into Kubernetes secrets.  It does so by registering a Custom Resource Definition of BitwardenSecret into the cluster.  It will listen for new BitwardenSecrets registered on the cluster and then synchronize on a configurable interval.

## Getting Started

You will need a Kubernetes cluster to run against. We recommend [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.

**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out

1. Install the CRDs into the cluster using `make install` or by using the Visual Studio Task called "apply-crd" from the "Tasks: Run Task" in the command palette.

1. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running) via `make run`.  To debug the code, just hit F5

**NOTE:** You can also run this in one step by running: `make install run`

### Uninstall CRDs

To delete the CRDs from the cluster:

```sh
make uninstall
```

### Running on the cluster

1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

1. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/sm-operator:tag
```

1. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/sm-operator:tag
```

### Undeploy controller

UnDeploy the controller from the cluster:

```sh
make undeploy
```

## Contributing

// TODO(user): Add detailed information on how you would like others to contribute to this project

### Modifying the API definitions

If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

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
