#!/usr/bin/env bash
apt-get update
apt-get install -y kubernetes-client musl-tools # kubectl
kind delete cluster --name sm-operator && kind create cluster --name sm-operator --config .devcontainer/common/kind-config.yaml
# kind export kubeconfig

PATH="$PATH:/usr/local/go/bin" make setup
PATH="$PATH:/usr/local/go/bin" make install

# shellcheck disable=SC2016
echo '
devcontainer setup complete!

Be sure to set the following environment variables in the ".env" file:
BWS_ACCESS_TOKEN=
BW_API_URL=
BW_IDENTITY_API_URL=

And run the following to set the Bitwarden access token secret before attempting to create a BitwardenSecret object:
kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="$BWS_ACCESS_TOKEN"
'
