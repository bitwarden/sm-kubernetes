# Bitwarden Secrets Manager Kubernetes Operator

This Operator is a tool for teams to integrate Bitwarden Secrets Manager into their Kubernetes workflows seamlessly.

## Getting Started

To get started developing, please install the following software.  You do not have to install the recommendations, but it is advised for testing.

### Pre-requisites

* [Go](https://go.dev/dl/) version 1.20 or 1.21
* [Operator-SDK](https://sdk.operatorframework.io/docs/installation/#install-from-github-release)
* [Make](https://www.gnu.org/software/make/)
* [Visual Studio Code Go Extension](https://marketplace.visualstudio.com/items?itemName=golang.go)
* [kubectl](https://kubernetes.io/docs/tasks/tools/)
* [Docker](https://www.docker.com/) or [Podman](https://podman.io/) or another container engine
* [Build Bitwarden SDK libbitwarden_c.so binary](https://github.com/bitwarden/sdk) copied to /usr/lib (NOTE: We will create a Makefile entry to download these automatically in the future)
* A [Bitwarden Organization with Secrets Manager](https://bitwarden.com/help/sign-up-for-secrets-manager/).  You will need the organization ID GUID for your organization.
* An [access token](https://bitwarden.com/help/access-tokens/) for a Secrets Manager service account tied to the projects you want to pull.

### Recommended

* A [Kind Cluster](https://kind.sigs.k8s.io/docs/user/quick-start/) with Kubectl pointed to it as the current context for local development.
* [Bitwarden Secrets Manager CLI](https://github.com/bitwarden/sdk/releases)

### Development

Open Visual Studio Code to the [src/sm-operator](src/sm-operator) subfolder.  Opening here will allow all of the Go language extensions, tasks, and launch settings to work correctly. Please note that the Visual Studio Code debugger for Go does not work correctly if you open your workspace via a symlink anywhere in the path.  For debugging to work, you should open it from the full path of the repository.

The [README](src/sm-operator/README.md) has further details for interacting with and debugging the code.