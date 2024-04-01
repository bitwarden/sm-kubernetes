# Bitwarden Secrets Manager Kubernetes Operator

This Operator is a tool for teams to integrate Bitwarden Secrets Manager into their Kubernetes workflows seamlessly.

## Getting Started

To get started developing, please install the following software.  You do not have to install the recommendations, but it is advised for testing.

### Pre-requisites

A Visual Studio Code Dev Container is provided for development purposes, and handles the setup of all of these pre-requisites.  It is strongly recommended that you use the Dev Container, especially on Mac and Windows.  The only requirements for the Dev Container are:

* [Visual Studio Code](https://code.visualstudio.com/)
* [Docker](https://www.docker.com/) - Podman is not currently supported with our Dev Container
* [Visual Studio Code Dev Containers Extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)

You will need to open a Visual Studio Code instance at the [src/sm-operator](src/sm-operator) subfolder level to use the Dev Container.

For manual Linux setups:

* [Go](https://go.dev/dl/) version 1.20 or 1.21
* [Operator-SDK](https://sdk.operatorframework.io/docs/installation/#install-from-github-release)
* [musl-gcc](https://wiki.musl-libc.org/getting-started.html)
* [Make](https://www.gnu.org/software/make/)
* [Visual Studio Code Go Extension](https://marketplace.visualstudio.com/items?itemName=golang.go)
* [kubectl](https://kubernetes.io/docs/tasks/tools/)
* [Docker](https://www.docker.com/) or [Podman](https://podman.io/) or another container engine
* [Download the appropriate libbitwarden_c binary](https://github.com/bitwarden/sdk) for your OS and architecture to [src/sm-operator/bw-sdk/internal/cinterface/lib](src/sm-operator/bw-sdk/internal/cinterface/lib).  This can be done using `make setup`
* A [Bitwarden Organization with Secrets Manager](https://bitwarden.com/help/sign-up-for-secrets-manager/).  You will need the organization ID GUID for your organization.
* An [access token](https://bitwarden.com/help/access-tokens/) for a Secrets Manager service account tied to the projects you want to pull.
* A [Kind Cluster](https://kind.sigs.k8s.io/docs/user/quick-start/) or other local Kubernetes environment with Kubectl pointed to it as the current context for local development.

### Recommended

* [Bitwarden Secrets Manager CLI](https://github.com/bitwarden/sdk/releases)

### Development

Open Visual Studio Code to the [src/sm-operator](src/sm-operator) subfolder.  Opening here will allow all of the Go language extensions, tasks, and launch settings to work correctly. Please note that the Visual Studio Code debugger for Go does not work correctly if you open your workspace via a symlink anywhere in the path.  For debugging to work, you should open it from the full path of the repository.

The [README](src/sm-operator/README.md) has further details for interacting with and debugging the code.
