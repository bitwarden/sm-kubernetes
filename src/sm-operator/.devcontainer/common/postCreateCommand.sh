#!/usr/bin/env bash
apt-get update
apt-get install -y kubernetes-client musl-tools # kubectl
kind create cluster --config .devcontainer/common/kind-config.yaml
# kind export kubeconfig

PATH="$PATH:/usr/local/go/bin" make setup
PATH="$PATH:/usr/local/go/bin" make install

# shellcheck disable=SC2016
echo '
devcontainer setup complete!

Be sure to set the following environment variables:
export BWS_ACCESS_TOKEN=
export BW_API_URL=
export BW_IDENTITY_API_URL=

And run the following before attempting to set the Bitwarden access token secret:
kubectl create secret generic bw-auth-token -n some-namespace --from-literal=token="$BWS_ACCESS_TOKEN"
'
