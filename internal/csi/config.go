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
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// defaultAPIURL and defaultIdentityURL mirror the fallbacks applied by
// cmd.GetSettings when the corresponding BW_API_URL / BW_IDENTITY_API_URL
// environment variables are unset, so that the apiUrl/identityUrl
// SecretProviderClass parameters default the same way as the controller's
// environment-based configuration.
const (
	defaultAPIURL      = "https://api.bitwarden.com"
	defaultIdentityURL = "https://identity.bitwarden.com"
)

// maxFilePermission is the largest valid octal permission bits value
// (rwxrwxrwx) that a filePermission entry may specify.
const maxFilePermission = 0o777

// ObjectEntry is a single entry in the "objects" SecretProviderClass
// parameter. It mirrors the semantics of api/v1.SecretMap: a Bitwarden
// Secrets Manager secret is identified either by its ID (BwSecretID) or by
// its name (SecretName), and is written out as a file named FileName in the
// CSI volume. FilePermission optionally overrides the permission bits the
// file is created with.
type ObjectEntry struct {
	// BwSecretID is the Secrets Manager secret ID (UUID) to mount. Exactly
	// one of BwSecretID or SecretName must be set.
	BwSecretID string `json:"bwSecretId,omitempty"`
	// SecretName is the Secrets Manager secret name to mount. Exactly one of
	// BwSecretID or SecretName must be set. Names must be unique across all
	// secrets accessible by the machine account, mirroring the
	// UseSecretNames semantics on BitwardenSecretSpec.
	SecretName string `json:"secretName,omitempty"`
	// FileName is the name of the file written into the CSI volume for this
	// secret. Required.
	FileName string `json:"fileName"`
	// FilePermission optionally overrides the permission bits (as an octal
	// string, e.g. "0600") the file is created with. Optional.
	FilePermission string `json:"filePermission,omitempty"`
}

// Parameters is the parsed and validated representation of the
// SecretProviderClass.spec.parameters map understood by this provider.
type Parameters struct {
	// OrganizationID is the Bitwarden organization ID secrets are pulled
	// from. Required.
	OrganizationID string
	// Objects is the set of Secrets Manager secrets to mount as files.
	// Required; at least one entry must be present.
	Objects []ObjectEntry
	// APIURL is the Bitwarden API URL to use, defaulting to
	// defaultAPIURL when the apiUrl parameter is unset.
	APIURL string
	// IdentityURL is the Bitwarden Identity URL to use, defaulting to
	// defaultIdentityURL when the identityUrl parameter is unset.
	IdentityURL string
}

// ParseParameters parses and validates attributesJSON, the JSON-encoded
// object the Secrets Store CSI Driver sends as MountRequest.Attributes. This
// JSON object is built by the driver by marshaling
// SecretProviderClass.spec.parameters (a map[string]string) merged with a
// handful of driver-injected keys (pod name/namespace, etc.), so every value
// at the top level of attributesJSON is a JSON string.
//
// The "objects" parameter itself holds a JSON-encoded array of ObjectEntry
// values, since map[string]string values cannot natively hold nested
// structures.
//
// It returns a clear, descriptive error identifying the missing or
// malformed field on any validation failure.
func ParseParameters(attributesJSON string) (*Parameters, error) {
	if strings.TrimSpace(attributesJSON) == "" {
		return nil, fmt.Errorf("parameters: attributes is empty")
	}

	var attrs map[string]string
	if err := json.Unmarshal([]byte(attributesJSON), &attrs); err != nil {
		return nil, fmt.Errorf("parameters: failed to parse attributes JSON: %w", err)
	}

	orgID := strings.TrimSpace(attrs["organizationId"])
	if orgID == "" {
		return nil, fmt.Errorf("parameters: organizationId is required")
	}

	objects, err := parseObjects(attrs["objects"])
	if err != nil {
		return nil, err
	}

	apiURL, err := resolveURL(attrs["apiUrl"], defaultAPIURL, "apiUrl")
	if err != nil {
		return nil, err
	}

	identityURL, err := resolveURL(attrs["identityUrl"], defaultIdentityURL, "identityUrl")
	if err != nil {
		return nil, err
	}

	return &Parameters{
		OrganizationID: orgID,
		Objects:        objects,
		APIURL:         apiURL,
		IdentityURL:    identityURL,
	}, nil
}

// parseObjects parses and validates the "objects" parameter value, a
// JSON-encoded array of ObjectEntry values.
func parseObjects(raw string) ([]ObjectEntry, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("parameters: objects is required")
	}

	var entries []ObjectEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("parameters: objects is not a valid JSON array: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("parameters: objects must contain at least one entry")
	}

	for i, entry := range entries {
		if err := validateObjectEntry(entry); err != nil {
			return nil, fmt.Errorf("parameters: objects[%d]: %w", i, err)
		}
	}

	return entries, nil
}

// validateObjectEntry validates a single ObjectEntry, mirroring the
// bwSecretId/secretName identification semantics of api/v1.SecretMap.
func validateObjectEntry(entry ObjectEntry) error {
	bwSecretID := strings.TrimSpace(entry.BwSecretID)
	secretName := strings.TrimSpace(entry.SecretName)

	switch {
	case bwSecretID == "" && secretName == "":
		return fmt.Errorf("exactly one of bwSecretId or secretName is required")
	case bwSecretID != "" && secretName != "":
		return fmt.Errorf("only one of bwSecretId or secretName may be set, got both")
	}

	if strings.TrimSpace(entry.FileName) == "" {
		return fmt.Errorf("fileName is required")
	}

	if entry.FilePermission != "" {
		if _, err := parseFilePermission(entry.FilePermission); err != nil {
			return fmt.Errorf("filePermission %q is invalid: %w", entry.FilePermission, err)
		}
	}

	return nil
}

// parseFilePermission parses permission as an octal file mode string (e.g.
// "0600" or "600") and validates it is within the valid permission bits
// range.
func parseFilePermission(permission string) (uint32, error) {
	value, err := strconv.ParseUint(permission, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("must be an octal permission string: %w", err)
	}

	if value > maxFilePermission {
		return 0, fmt.Errorf("must be between 0 and 0777")
	}

	return uint32(value), nil
}

// resolveURL applies the fallback default when raw is empty, and otherwise
// validates raw is an absolute URL with both a scheme and a host, mirroring
// the validation cmd.GetSettings performs on BW_API_URL /
// BW_IDENTITY_API_URL.
func resolveURL(raw, fallback, fieldName string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}

	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		if err == nil {
			err = fmt.Errorf("missing scheme or host")
		}

		return "", fmt.Errorf("parameters: %s is not a valid URL: %w", fieldName, err)
	}

	return raw, nil
}
