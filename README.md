# Bitwarden Secrets Manager Kubernetes Operator

This Operator is a tool for teams to integrate Bitwarden Secrets Manager into their Kubernetes workflows seamlessly.

## Description

The sm-operator uses a [controller](internal/controller/bitwardensecret_controller.go) to synchronize Bitwarden Secrets into Kubernetes secrets. It does so by registering a Custom Resource Definition of BitwardenSecret into the cluster. It will listen for new BitwardenSecrets registered on the cluster and then synchronize on a configurable interval.

## Getting Started

To get started developing, please install the following software. You do not have to install the recommendations, but it is advised for testing.

You will need a Kubernetes cluster to run against. We recommend [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.

**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

Run `make setup` to generate an example `.env` file. If you are using the Dev Container, this step has already been completed for you.

### Pre-requisites

A Visual Studio Code Dev Container is provided for development purposes, and handles the setup of all of these pre-requisites. It is strongly recommended that you use the Dev Container, especially on Mac and Windows. The only requirements for the Dev Container are:

-   [Visual Studio Code](https://code.visualstudio.com/)
-   [Docker](https://www.docker.com/) - Podman is not currently supported with our Dev Container
-   [Visual Studio Code Dev Containers Extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)

You will need to open Visual Studio Code at the repository root to use the Dev Container.

For manual Linux setups:

-   [Go](https://go.dev/dl/) version 1.20 or 1.21
-   [Operator-SDK](https://sdk.operatorframework.io/docs/installation/#install-from-github-release)
-   [musl-gcc](https://wiki.musl-libc.org/getting-started.html)
-   [Make](https://www.gnu.org/software/make/)
-   [Visual Studio Code Go Extension](https://marketplace.visualstudio.com/items?itemName=golang.go)
-   [kubectl](https://kubernetes.io/docs/tasks/tools/)
-   [Docker](https://www.docker.com/) or [Podman](https://podman.io/) or another container engine
-   A [Bitwarden Organization with Secrets Manager](https://bitwarden.com/help/sign-up-for-secrets-manager/). You will need the organization ID GUID for your organization.
-   An [access token](https://bitwarden.com/help/access-tokens/) for a Secrets Manager machine account tied to the projects you want to pull.
-   A [Kind Cluster](https://kind.sigs.k8s.io/docs/user/quick-start/) or other local Kubernetes environment with Kubectl pointed to it as the current context for local development.

### Recommended

-   [Bitwarden Secrets Manager CLI](https://github.com/bitwarden/sdk/releases)

### Development

Open the project in Visual Studio Code. Please develop in the DevContainer provided. Please note that the Visual Studio Code debugger for Go does not work correctly if you open your workspace via a symlink anywhere in the path. For debugging to work, you should open it from the full path of the repository.

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster. The controller ([internal/controller/bitwardensecret_controller.go](internal/controller/bitwardensecret_controller.go)) is where the main synchronization/reconciliation takes place. The types file ([api/v1/bitwardensecret_types.go](api/v1/bitwardensecret_types.go)) specifies the structure of the Custom Resource Definition used throughout the controller, as well as the manifest structure.

The [config](config/) directory contains the generated manifest definitions for deployment and testing of the operator into Kubernetes.

## Modifying the API definitions

If you are editing the API definitions via [api/v1/bitwardensecret_types.go](api/v1/bitwardensecret_types.go), re-generate the manifests such as the Custom Resource Definition using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Debugging

1. Install the Custom Resource Definition into the cluster using `make install` or by using the Visual Studio Task called "apply-crd" from the "Tasks: Run Task" in the command palette.

1. To debug the code, just hit F5. You can also use `make run` at the command line to run without debugging.

**NOTE:** You can also run this in one step by running: `make install run`

### Configuration settings

A `.env` file will be created under this workspace's root directory once the Dev Container is created or `make setup` has been run. The following environment variable settings can
be updated to change the behavior of the operator:

-   **BW_API_URL** - Sets the Bitwarden API URL that the Secrets Manager SDK uses. This is useful for self-host scenarios, as well as hitting European servers
-   **BW_IDENTITY_API_URL** - Sets the Bitwarden Identity service URL that the Secrets Manager SDK uses. This is useful for self-host scenarios, as well as hitting European servers
-   **BW_SECRETS_MANAGER_STATE_PATH** - Sets the base path where Secrets Manager SDK stores its state files
-   **BW_SECRETS_MANAGER_REFRESH_INTERVAL** - Specifies the refresh interval in seconds for syncing secrets between Secrets Manager and K8s secrets. The minimum value is 180.

### BitwardenSecret

Our operator is designed to look for the creation of a custom resource called a BitwardenSecret. Think of the BitwardenSecret object as the synchronization settings that will be used by the operator to create and synchronize a Kubernetes secret. This Kubernetes secret will live inside of a namespace and will be injected with the data available to a Secrets Manager machine account. The resulting Kubernetes secret will include all secrets that a specific machine account has access to. The sample manifest ([config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml)) gives the basic structure of the BitwardenSecret. The key settings that you will want to update are listed below:

-   **metadata.name**: The name of the BitwardenSecret object you are deploying
-   **spec.organizationId**: The Bitwarden organization ID you are pulling Secrets Manager data from
-   **spec.secretName**: The name of the Kubernetes secret that will be created and injected with Secrets Manager data.
-   **spec.authToken**: Configuration for the Secrets Manager machine account authorization token. By default, looks for the secret in the same namespace as the BitwardenSecret, but can optionally specify a different namespace.

Secrets Manager does not guarantee unique secret names across projects, so by default secrets will be created with the Secrets Manager secret UUID used as the key. To make your generated secret easier to use, you can create a map of Bitwarden Secret IDs to Kubernetes secret keys. The generated secret will replace the Bitwarden Secret IDs with the mapped friendly name you provide. Below are the map settings available:

-   **bwSecretId**: This is the UUID of the secret in Secrets Manager. This can found under the secret name in the Secrets Manager web portal or by using the [Bitwarden Secrets Manager CLI](https://github.com/bitwarden/sdk/releases).
-   **secretKeyName**: The resulting key inside the Kubernetes secret that replaces the UUID

Note that the custom mapping is made available on the generated secret for informational purposes in the `k8s.bitwarden.com/custom-map` annotation.

#### Creating a BitwardenSecret object

To test the operator, we will create a BitwardenSecret object. But first, we will need to create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object:

```shell
kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"
```

Next, create an instance of BitwardenSecret. An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml):

```shell
kubectl apply -n some-namespace -f config/samples/k8s_v1_bitwardensecret.yaml
```

### Uninstall Custom Resource Definition

To delete the CRDs from the cluster:

```sh
make uninstall
```

## Testing the container

The following sections describe how to test the container image itself. Up to this point the operator has been tested outside of the cluster. These next steps will allow us to test the operator running inside of the cluster. Custom configuration of URLs, refresh interval, and state path is handled by updating the environment variables in [config/manager/manager.yaml](config/manager/manager.yaml) when working with the container.

### Running on Kind cluster

1. Build and push your image directly to Kind by using the Visual Studio Code Command Palette. Open the palette and select Tasks: Run Task and select "docker-build" followed by "kind-push".

1. Deploy the Kubernetes objects to Kind by using the Visual Studio Code Command Palette. Open the palette (F1) and select Tasks: Run Task and select "deploy".

1. Create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object: `kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"`

1. Create an instances of BitwardenSecret. An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml): `kubectl apply -n some-namespace -f config/samples/k8s_v1_bitwardensecret.yaml`

### Alternative: Running on a cluster using a registry

1. Build and push your image to the registry location specified by `IMG`: `make docker-build docker-push IMG=<some-registry>/sm-operator:tag`

1. Deploy the controller to the cluster with the image specified by `IMG`: `make deploy IMG=<some-registry>/sm-operator:tag`

1. Create a secret to house the Secrets Manager authentication token in the namespace where you will be creating your BitwardenSecret object: `kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="<Auth-Token-Here>"`

1. Create an instance of BitwardenSecret. An example can be found in [config/samples/k8s_v1_bitwardensecret.yaml](config/samples/k8s_v1_bitwardensecret.yaml): `kubectl apply -n some-namespace -f config/samples/k8s_v1_bitwardensecret.yaml`

### Undeploy controller

To "UnDeploy" the controller from the cluster after testing, run:

```sh
make undeploy
```

### Unit test

Unit tests are currently found in the following files:

-   internal/controller/suite_test.go

-   cmd/suite_test.go

To run the unit tests, run `make test` from the root directory of this workspace. To debug the unit tests, click on the file you would like to debug. In the `Run and Debug` tab in Visual Studio Code, change the launch configuration from "Debug" to "Test current file", and then press F5. 

**NOTE: Using the Visual Studio Code "Testing" tab may not work OOB due to VS Code not linking the static binaries correctly.  The solution is to perform the following tasks***

Update VSCode Settings for Tests:

* Open VSCode settings (Ctrl+, or Cmd+,).
* Search for go.testFlags.
* Add the following to the go.testFlags array:

```json

["-ldflags=-extldflags=-lm"]

```

This tells the Go test runner to include the linker flag for all test commands.

