# Bitwarden CSI Provider

This directory packages the Bitwarden Secrets Manager provider
(`cmd/csi-provider`, implemented by [`internal/csi`](../../internal/csi)) for
deployment as a [secrets-store-csi-driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver)
provider. The provider is a gRPC server that the driver calls over a Unix
Domain Socket to resolve `SecretProviderClass` objects with
`spec.provider: bitwarden` into files backed by Bitwarden Secrets Manager
secrets.

This directory does **not** install the driver itself. The driver is a
separate, cluster-wide component (see
[Prerequisite: the upstream driver](#prerequisite-the-upstream-driver)
below).

## Contents

- [`daemonset.yaml`](daemonset.yaml), [`rbac.yaml`](rbac.yaml),
  [`kustomization.yaml`](kustomization.yaml) -- plain manifests deploying the
  provider as a DaemonSet, for use directly or via `kubectl kustomize` /
  `kustomize build`.
- [`secretproviderclass-sample.yaml`](secretproviderclass-sample.yaml) -- a
  sample `SecretProviderClass` showing every parameter this provider
  understands.
- [`chart/`](chart) -- the same DaemonSet packaged as a Helm chart, with the
  upstream driver as an optional chart dependency. See
  [`chart/README.md`](chart/README.md).

## Prerequisite: the upstream driver

Rotation depends entirely on the upstream secrets-store-csi-driver, not on
this provider. The driver must be **>= v1.6.0** (the version this repository
builds and tests against; see `go.mod`) for `RequiresRepublish`-based
rotation to work with this provider: on older driver versions, mounted
secrets are fetched once at pod start and never refreshed. Install it however
suits your cluster:

- via its own Helm chart / manifests, cluster-wide and independent of this
  provider, or
- via [`chart/`](chart)'s optional dependency (`--set
  secrets-store-csi-driver.install=true`).

Either way, the driver must be started with `--enable-secret-rotation` and a
`--rotation-poll-interval` for periodic rotation to actually run; see the
[upstream rotation docs](https://secrets-store-csi-driver.sigs.k8s.io/topics/rotation).

## Why this provider needs almost no RBAC

The `Mount` RPC this provider implements ([`internal/csi/server.go`](../../internal/csi/server.go))
only ever reads two things the driver hands it directly on the call:

- the `SecretProviderClass.spec.parameters` (`organizationId`, `objects`, ...)
- the Secrets Manager access token resolved from `nodePublishSecretRef`

The upstream driver -- not this provider -- resolves `nodePublishSecretRef`
against the core `v1.Secret` before ever calling us. This provider itself
never calls the Kubernetes API, so [`rbac.yaml`](rbac.yaml) defines only a
`ServiceAccount` (with its token's auto-mount disabled) and no
Role/ClusterRole/RoleBinding/ClusterRoleBinding at all. See the comments in
that file if a future change to this provider needs to call the Kubernetes
API -- grant the minimum verbs required at that point.

## Keeping the raw manifests and the Helm chart in sync

[`daemonset.yaml`](daemonset.yaml) / [`rbac.yaml`](rbac.yaml) and
[`chart/templates/daemonset.yaml`](chart/templates/daemonset.yaml) /
[`chart/templates/serviceaccount.yaml`](chart/templates/serviceaccount.yaml)
describe the same DaemonSet/ServiceAccount/RBAC posture twice, with no shared
source of truth between the two formats. Security-relevant fields
(`securityContext`, `capabilities`, `resources`, `tolerations`,
`priorityClassName`, RBAC scope, volumes/volumeMounts) are hand-duplicated
across both, and each file carries a `KEPT IN SYNC WITH` comment pointing at
its counterpart. When changing any of those fields in one, mirror the change
in the other, and confirm the two still render equivalently with:

```console
diff <(kubectl kustomize config/csi-provider) \
     <(helm template sync config/csi-provider/chart --namespace kube-system \
         --set fullnameOverride=bitwarden-csi-provider)
```

(the `helm template` output additionally has Helm's own `app.kubernetes.io/*`
release labels, which is expected -- the diff should otherwise be empty.)

## Deploying the raw manifests

```console
kubectl apply -k config/csi-provider
```

Or, to set your own image before applying, edit the `images:` entry in
[`kustomization.yaml`](kustomization.yaml), which currently points at a
placeholder (`localhost/sm-csi-provider:1.0.0`) built via
`docker build --target csi-provider` from the [repo root Dockerfile](../../Dockerfile).

## Deploying the Helm chart

See [`chart/README.md`](chart/README.md).

## Building the provider image

The provider links the same Bitwarden SDK C library
(`github.com/bitwarden/sdk-go/v2`) as the controller manager, so it is built
the same way: `CGO_ENABLED=1` with `musl-gcc`, statically linked. It's a
build target in the repo root [`Dockerfile`](../../Dockerfile), which
defaults to still building the controller manager image when no `--target`
is given:

```console
docker build --target csi-provider -t <your-registry>/sm-csi-provider:<tag> .
```

## Using the sample SecretProviderClass

[`secretproviderclass-sample.yaml`](secretproviderclass-sample.yaml) shows
every `spec.parameters` key this provider understands
(see [`internal/csi/config.go`](../../internal/csi/config.go)'s
`ParseParameters`). A consuming Pod references it, and provides the Secrets
Manager access token via `nodePublishSecretRef`, roughly like:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bitwarden-token
type: Opaque
stringData:
  # A Secrets Manager machine account access token. Never commit a real
  # token to source control; create this Secret out-of-band (e.g. from your
  # own secrets pipeline).
  token: "<access-token>"
---
apiVersion: v1
kind: Pod
metadata:
  name: example
spec:
  containers:
    - name: example
      image: busybox
      command: ["sleep", "infinity"]
      volumeMounts:
        - name: bitwarden-secrets
          mountPath: /mnt/secrets
          readOnly: true
  volumes:
    - name: bitwarden-secrets
      csi:
        driver: secrets-store.csi.k8s.io
        readOnly: true
        volumeAttributes:
          secretProviderClass: bitwarden-secrets
        nodePublishSecretRef:
          name: bitwarden-token
```
