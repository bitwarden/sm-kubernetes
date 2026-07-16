# bitwarden-csi-provider

Helm chart for the Bitwarden Secrets Manager provider for the Kubernetes
[secrets-store-csi-driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver).
It installs a DaemonSet that runs the provider (`cmd/csi-provider`) on every
node, plus the ServiceAccount it runs under.

See [../README.md](../README.md) for the full picture, including the
non-chart raw manifests (`../*.yaml`), the sample `SecretProviderClass`
(`../secretproviderclass-sample.yaml`), and why the driver must be
**>= v1.6.0** for rotation to work.

## Prerequisites

- The upstream secrets-store-csi-driver, **>= v1.6.0**. This chart does
  *not* install it by default (see below).

## Installing

This chart declares the upstream `secrets-store-csi-driver` chart as an
optional dependency, so `helm dependency update` must be run once before
installing or templating, even if you don't intend to use it:

```console
helm dependency update ./config/csi-provider/chart
```

If the upstream driver is already installed elsewhere in the cluster (the
common case), just install this chart on its own:

```console
helm install bitwarden-csi-provider ./config/csi-provider/chart \
  --namespace kube-system \
  --set image.repository=<your-registry>/sm-csi-provider \
  --set image.tag=<your-tag>
```

To also install the upstream driver as part of this release, set
`secrets-store-csi-driver.install=true`; any other
`secrets-store-csi-driver.*` values are passed straight through to that
subchart:

```console
helm install bitwarden-csi-provider ./config/csi-provider/chart \
  --namespace kube-system \
  --set secrets-store-csi-driver.install=true \
  --set image.repository=<your-registry>/sm-csi-provider \
  --set image.tag=<your-tag>
```

## Values

| Key | Default | Description |
| --- | --- | --- |
| `image.repository` | `localhost/sm-csi-provider` | Provider image repository. Build it with `docker build --target csi-provider` from the repo root Dockerfile. |
| `image.tag` | `1.0.0` | Provider image tag. |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `serviceAccount.create` | `true` | Whether to create a ServiceAccount. No Role/ClusterRole is ever created; the provider needs none (see [../README.md](../README.md)). |
| `serviceAccount.name` | `""` | ServiceAccount name; defaults to the release fullname. |
| `socketDir` | `/etc/kubernetes/secrets-store-csi-providers` | hostPath shared with the upstream driver's node pods; the provider's Unix Domain Socket is created here. |
| `priorityClassName` | `system-node-critical` | Set to `""` to omit. |
| `tolerations` | tolerate everything | Runs on every node, including tainted control-plane nodes. |
| `resources` | `500m`/`128Mi` limits, `10m`/`64Mi` requests | Container resources. |
| `extraArgs` | `[]` | Extra args appended after `--endpoint`. |
| `secrets-store-csi-driver.install` | `false` | Install the upstream driver as a subchart alongside this provider. |

See [values.yaml](values.yaml) for the complete set of configurable values.
