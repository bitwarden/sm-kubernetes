# sm-operator

The sm-operator project is a Kubernetes operator designed to synchronize Bitwarden Secrets Manager secrets into K8s secrets.

## Description

The sm-operator uses a [controller](internal/controller/bitwardensecret_controller.go) to synchronize Bitwarden Secrets into Kubernetes secrets.  It does so by registering a Custom Resource Definition of BitwardenSecret into the cluster.  It will listen for new BitwardenSecrets registered on the cluster and then synchronize on a configurable interval.

## Getting Started

You will need a Kubernetes cluster to run against. We recommend [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.

**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

This project uses the Secrets Manager golang SDK.  This SDK requires some binaries exist inside the project.  Run `make setup` to download the appropriate binaries into the project.  If you are using the Dev Container, this step has already been completed for you.

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.  The controller ([internal/controller/bitwardensecret_controller.go](internal/controller/bitwardensecret_controller.go)) is where the main synchronization/reconciliation takes place.  The types file ([api/v1/bitwardensecret_types.go](api/v1/bitwardensecret_types.go)) specifies the structure of the Custom Resource Definition used throughout the controller, as well as the manifest structure.

The [config](config/) directory contains the generated manifest definitions for deployment and testing of the operator into kubernetes.

## Modifying the API definitions

If you are editing the API definitions via [api/v1/bitwardensecret_types.go](api/v1/bitwardensecret_types.go), re-generate the manifests such as the custom resource definition using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Debugging

1. Install the Custom Resource Definition into the cluster using `make install` or by using the Visual Studio Task called "apply-crd" from the "Tasks: Run Task" in the command palette.

1. To debug the code, just hit F5.  You can also use `make run` at the command line to run without debugging.

**NOTE:** You can also run this in one step by running: `make install run`

### Configuration settings

A `.env` file will be created underthis workspace's root directory once the Dev Container is created or `make setup` has been run. The following environment variable settings can
be updated to change the behavior of the operator:

* **BW_API_URL** - Sets the Bitwarden API URL that the Secrets Manager SDK uses. This is useful for self-host scenarios, as well as hitting European servers
* **BW_IDENTITY_API_URL** - Sets the Bitwarden Identity service URL that the Secrets Manager SDK uses. This is useful for self-host scenarios, as well as hitting European servers
* **BW_SECRETS_MANAGER_STATE_PATH** - Sets the base path where Secrets Manager SDK stores its state files
* **BW_SECRETS_MANAGER_REFRESH_INTERVAL** - Specifies the refresh interval in seconds for syncing secrets between Secrets Manager and K8s secrets. The minimum value is 180.

### BitwardenSecret

Our operator is designed to look for the creation of a custom resource called a BitwardenSecret.  Think of the BitwardenSecret object as the synchronization settings that will be used by the operator to create and synchronize a Kubernetes secret. This Kubernetes secret will live inside of a namespace and will be injected with the data available to a Secrets Manager machine account. The resulting Kubernetes secret will include all secrets that a specific machine account has access to. The sample manifest ([config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml)) gives the basic structure of the BitwardenSecret.  The key settings that you will want to update are listed below:

* **metadata.name**: The name of the BitwardenSecret object you are deploying
* **spec.organizationId**: The Bitwarden organization ID you are pulling Secrets Manager data from
* **spec.secretName**: The name of the Kubernetes secret that will be created and injected with Secrets Manager data.
* **spec.authToken**: The name of a secret inside of the Kubernetes namespace that the BitwardenSecrets object is being deployed into that contains the Secrets Manager machine account authorization token being used to access secrets.

Secrets Manager does not guarantee unique secret names across projects, so by default secrets will be created with the Secrets Manager secret UUID used as the key.  To make your generated secret easier to use, you can create a map of Bitwarden Secret IDs to Kubernetes secret keys.  The generated secret will replace the Bitwarden Secret IDs with the mapped friendly name you provide.  Below are the map settings available:

* **bwSecretId**: This is the UUID of the secret in Secrets Manager.  This can found under the secret name in the Secrets Manager web portal or by using the [Bitwarden Secrets Manager CLI](https://github.com/bitwarden/sdk/releases).
* **secretKeyName**: The resulting key inside the Kubernetes secret that replaces the UUID

Note that the custom mapping is made available on the generated secret for informational purposes in the `k8s.bitwarden.com/custom-map` annotation.

#### Creating a BitwardenSecret object

To test the operator, we will create a BitwardenSecret object.  But first, we will need to create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object:

```shell
kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"
```

Next, create an instances of BitwardenSecret.  An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml):

```shell
kubectl apply -n some-namespace -f config/samples/k8s_v1_bitwardensecret.yaml
```

### Uninstall Custom Resource Definition

To delete the CRDs from the cluster:

```sh
make uninstall
```

## Testing the container

The following sections describe how to test the container image itself.  Up to this point the operator has been tested outside of the cluster.  These next steps will allow us to test the operator running inside of the cluster. Custom configuration of URLs, refresh interval, and state path is handled by updating the environment variables in [config/manager/manager.yaml](config/manager/manager.yaml) when working with the container.

### Running on Kind cluster

1. Build and push your image directly to Kind by using the Visual Studio Code Command Palette.  Open the palette and select Tasks: Run Task and select "docker-build" followed by "kind-push".

1. Deploy the Kubernetes objects to Kind by using the Visual Studio Code Command Palette.  Open the palette  (F1) and select Tasks: Run Task and select "deploy".

1. Create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object: `kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"`

1. Create an instances of BitwardenSecret.  An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml):  `kubectl apply -n some-namespace -f config/samples/k8s_v1_bitwardensecret.yaml`

### Alternative: Running on a cluster using a registry

1. Build and push your image to the registry location specified by `IMG`: `make docker-build docker-push IMG=<some-registry>/sm-operator:tag`

1. Deploy the controller to the cluster with the image specified by `IMG`: `make deploy IMG=<some-registry>/sm-operator:tag`

1. Create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object: `kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"`

1. Create an instances of BitwardenSecret.  An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml):  `kubectl apply -n some-namespace -f config/samples/k8s_v1_bitwardensecret.yaml`

### Undeploy controller

To "UnDeploy" the controller from the cluster after testing, run:

```sh
make undeploy
```

### Unit test

Unit tests are current found in the following files:

* internal/controller/suite_test.go

* cmd/suite_test.go

To run the unit tests, run `make test` from the root directory of this workspace.  To debug the unit tests, click on the file you would like to debug.  In the `Run and Debug` tab in Visual Studio Code, change the lanch configuration from "Debug" to "Test current file", and then press F5.  **NOTE: Using the Visual Studio Code "Testing" tab does not currently work due to VS Code not linking the static binaries correctly.**

## Contributing

This project is open to public contribution, but you must follow the [Bitwarden Contribution Guidelines](https://contributing.bitwarden.com/).

## License

Source code in this repository is covered by one of two licenses: (i) the
GNU General Public License (GPL) v3.0 (ii) the Bitwarden License v1.0. The
default license throughout the repository is GPL v3.0 unless the header
specifies another license. Bitwarden Licensed code is found only in the
/bitwarden_license directory.

GPL v3.0:
<https://github.com/bitwarden/server/blob/main/LICENSE_GPL.txt>

Bitwarden License v1.0:
<https://github.com/bitwarden/server/blob/main/LICENSE_BITWARDEN.txt>

No grant of any rights in the trademarks, service marks, or logos of Bitwarden is
made (except as may be necessary to comply with the notice requirements as
applicable), and use of any Bitwarden trademarks must comply with Bitwarden
Trademark Guidelines
<https://github.com/bitwarden/server/blob/main/TRADEMARK_GUIDELINES.md>.
