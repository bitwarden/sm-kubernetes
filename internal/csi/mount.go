/*
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
*/

package csi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/bitwarden/sdk-go/v2"

	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"
)

// secretRefTokenKey is the key this provider expects within the
// nodePublishSecretRef Secret's data (forwarded by the driver as
// MountRequest.secrets) to hold the Secrets Manager access token. This
// mirrors the "token" key used by the existing controller's documented
// convention (see README.md), and is the DEFAULT and only supported
// authentication path: a per-pod access token supplied via
// nodePublishSecretRef. A shared/provider-level token is a lower-security
// fallback that is intentionally NOT implemented here and must never become
// the default.
const secretRefTokenKey = "token"

// extractAccessToken parses secretsJSON - MountRequest.secrets, the
// nodePublishSecretRef-resolved Secret data the driver forwards as a
// JSON-encoded map[string]string - and returns the Secrets Manager access
// token. It never logs or wraps the token value itself into an error string.
func extractAccessToken(secretsJSON string) (string, error) {
	if strings.TrimSpace(secretsJSON) == "" {
		return "", fmt.Errorf("mount: a nodePublishSecretRef secret containing a Secrets Manager access token is required")
	}

	var data map[string]string
	if err := json.Unmarshal([]byte(secretsJSON), &data); err != nil {
		return "", fmt.Errorf("mount: failed to parse nodePublishSecretRef secret data: %w", err)
	}

	token := strings.TrimSpace(data[secretRefTokenKey])
	if token == "" {
		return "", fmt.Errorf("mount: nodePublishSecretRef secret must contain a %q key with the Secrets Manager access token", secretRefTokenKey)
	}

	return token, nil
}

// buildMountFiles resolves each configured ObjectEntry against the synced
// Secrets Manager secrets (byID/byName, as produced by (*smCacheEntry).sync)
// and produces the CSI file list plus a stable per-file ObjectVersion. This
// is this provider's mapping/aliasing step, analogous to
// internal/sm.ApplySecretMap for the controller: each entry identifies a
// secret by ID or by name and aliases it to a file name.
//
// ObjectVersion.Version is derived from a hash of the secret's content, so
// it stays stable across Mount/rotation calls in which the underlying
// secret hasn't changed - letting the driver's rotation reconcile treat an
// unchanged secret as a no-op republish.
func buildMountFiles(objects []ObjectEntry, byID map[string]sdk.SecretResponse, byName map[string][]sdk.SecretResponse) ([]*pb.File, []*pb.ObjectVersion, error) {
	files := make([]*pb.File, 0, len(objects))
	versions := make([]*pb.ObjectVersion, 0, len(objects))

	for _, obj := range objects {
		secret, err := resolveSecret(obj, byID, byName)
		if err != nil {
			return nil, nil, err
		}

		mode := int32(maxFilePermission)

		if obj.FilePermission != "" {
			permission, err := parseFilePermission(obj.FilePermission)
			if err != nil {
				// ParseParameters already validates filePermission, so this
				// should be unreachable; handled defensively regardless.
				return nil, nil, fmt.Errorf("mount: file %q: %w", obj.FileName, err)
			}

			mode = int32(permission)
		}

		files = append(files, &pb.File{
			Path:     obj.FileName,
			Mode:     mode,
			Contents: []byte(secret.Value),
		})

		versions = append(versions, &pb.ObjectVersion{
			Id:      obj.FileName,
			Version: secretVersion(secret),
		})
	}

	return files, versions, nil
}

// resolveSecret finds the Secrets Manager secret an ObjectEntry identifies,
// by ID or by (possibly ambiguous) name.
func resolveSecret(obj ObjectEntry, byID map[string]sdk.SecretResponse, byName map[string][]sdk.SecretResponse) (sdk.SecretResponse, error) {
	if obj.BwSecretID != "" {
		secret, ok := byID[obj.BwSecretID]
		if !ok {
			return sdk.SecretResponse{}, fmt.Errorf("mount: secret with id %q (file %q) was not found or is not accessible to this machine account", obj.BwSecretID, obj.FileName)
		}

		return secret, nil
	}

	matches := byName[obj.SecretName]

	switch len(matches) {
	case 0:
		return sdk.SecretResponse{}, fmt.Errorf("mount: secret with name %q (file %q) was not found or is not accessible to this machine account", obj.SecretName, obj.FileName)
	case 1:
		return matches[0], nil
	default:
		return sdk.SecretResponse{}, fmt.Errorf("mount: secret name %q (file %q) is ambiguous: %d secrets share this name", obj.SecretName, obj.FileName, len(matches))
	}
}

// secretVersion derives a stable version string from a secret's content, so
// unchanged secrets always produce the same ObjectVersion.
//
// Deliberately, only secret.Value (the bytes actually written to the
// mounted file) feeds the hash. Every other SecretResponse field -
// RevisionDate and CreationDate in particular - is metadata that Secrets
// Manager may legitimately change independently of Value (e.g. touched by
// an unrelated update, or simply re-echoed on every Sync response), and is
// never written to the mounted file. Hashing any of those in would make
// object_versions churn on every driver rotation poll even though the
// mounted file contents never changed, defeating the whole point of
// RequiresRepublish-based rotation (see docs/rotation.md and
// TestBuildMountFilesVersionIgnoresVolatileMetadata).
func secretVersion(secret sdk.SecretResponse) string {
	sum := sha256.Sum256([]byte(secret.Value))
	return hex.EncodeToString(sum[:])
}
