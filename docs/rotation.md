# Secret rotation (CSI provider)

This provider (`internal/csi`, packaged in [`config/csi-provider`](../config/csi-provider))
does not implement rotation itself. Rotation of mounted secrets is a feature
of the upstream [secrets-store-csi-driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver),
and is entirely owned and driven by it.

## How it works

- The driver polls every mounted `SecretProviderClass` volume on a fixed
  interval and calls this provider's `Mount` RPC again for each one.
- This provider re-fetches the current secrets from Bitwarden Secrets
  Manager and returns a file list plus `object_versions` (one version string
  per file, see [`internal/csi/mount.go`](../internal/csi/mount.go)).
- The driver compares the returned `object_versions` against the versions it
  saw last time. If they're unchanged, it does nothing. If they differ, it
  rewrites the mounted files (a "republish") and, if configured, syncs the
  new values into any Kubernetes Secrets the `SecretProviderClass` also
  projects them into.

Because the driver -- not this provider -- decides whether to republish
based on `object_versions`, that value must be a pure function of the
secret's content: calling `Mount` twice with unchanged Secrets Manager data
must return identical `object_versions`, or the driver will republish on
every poll regardless of whether anything actually changed. See
`secretVersion` in [`internal/csi/mount.go`](../internal/csi/mount.go) and
its accompanying tests in
[`internal/csi/mount_test.go`](../internal/csi/mount_test.go) and
[`internal/csi/server_test.go`](../internal/csi/server_test.go).

## Enabling it

Rotation is off by default and must be explicitly enabled on the upstream
driver, not on this provider:

- Start the driver with `--enable-secret-rotation`.
- Set `--rotation-poll-interval` to how often it should re-check mounted
  volumes (the driver's own default is `2m`).

Without both flags, secrets are fetched once at pod start and never
refreshed for the lifetime of that mount, regardless of anything this
provider does.

See the [upstream rotation docs](https://secrets-store-csi-driver.sigs.k8s.io/topics/rotation)
for the authoritative reference.

## What this means in practice

- **Rotation is not instantaneous.** A changed secret in Bitwarden Secrets
  Manager is only picked up on the mounted volume's next poll, up to
  `--rotation-poll-interval` after the change (`2m` at the driver's default).
  There is no push/webhook path; nothing shortens this window.
- **The driver version matters.** `RequiresRepublish`-based rotation requires
  secrets-store-csi-driver **>= v1.6.0** (the version this repository builds
  and tests against; see `go.mod`). On older driver versions, rotation
  support does not exist: mounted secrets are fetched once at pod start and
  never refreshed, no matter how this provider's `Mount` RPC behaves.
- **A pod restart is not required.** Rotation rewrites the files already
  mounted into a running pod's volume; it does not (by itself) restart the
  pod. If a workload only reads secret files at startup, pair rotation with
  a mechanism that notices the file change (e.g. a sidecar that watches the
  file, or the driver's own Kubernetes Secret sync + a restart controller
  that watches that Secret).

See also [`config/csi-provider/README.md`](../config/csi-provider/README.md#prerequisite-the-upstream-driver)
for how this fits into deploying the provider itself.
