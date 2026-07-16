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
	"strings"
	"testing"
)

// buildAttributes marshals a map[string]string the way the Secrets Store
// CSI Driver marshals SecretProviderClass.spec.parameters (merged with
// driver-injected keys) into MountRequest.Attributes.
func buildAttributes(t *testing.T, params map[string]string) string {
	t.Helper()

	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal test attributes: %v", err)
	}

	return string(b)
}

func objectsJSON(t *testing.T, entries []ObjectEntry) string {
	t.Helper()

	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("failed to marshal test objects: %v", err)
	}

	return string(b)
}

func TestParseParametersValidInput(t *testing.T) {
	objects := []ObjectEntry{
		{BwSecretID: "11111111-1111-1111-1111-111111111111", FileName: "db-password", FilePermission: "0600"},
		{SecretName: "api-key", FileName: "api-key"},
	}

	attrs := buildAttributes(t, map[string]string{
		"organizationId": "22222222-2222-2222-2222-222222222222",
		"objects":        objectsJSON(t, objects),
		// driver-injected keys that are not part of our schema should be
		// tolerated and simply ignored.
		"csi.storage.k8s.io/pod.name":      "my-pod",
		"csi.storage.k8s.io/pod.namespace": "default",
	})

	params, err := ParseParameters(attrs)
	if err != nil {
		t.Fatalf("ParseParameters returned unexpected error: %v", err)
	}

	if params.OrganizationID != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("OrganizationID = %q, want %q", params.OrganizationID, "22222222-2222-2222-2222-222222222222")
	}

	if params.APIURL != defaultAPIURL {
		t.Errorf("APIURL = %q, want default %q", params.APIURL, defaultAPIURL)
	}

	if params.IdentityURL != defaultIdentityURL {
		t.Errorf("IdentityURL = %q, want default %q", params.IdentityURL, defaultIdentityURL)
	}

	if len(params.Objects) != 2 {
		t.Fatalf("len(Objects) = %d, want 2", len(params.Objects))
	}

	if params.Objects[0].BwSecretID != objects[0].BwSecretID || params.Objects[0].FileName != objects[0].FileName || params.Objects[0].FilePermission != objects[0].FilePermission {
		t.Errorf("Objects[0] = %+v, want %+v", params.Objects[0], objects[0])
	}

	if params.Objects[1].SecretName != objects[1].SecretName || params.Objects[1].FileName != objects[1].FileName {
		t.Errorf("Objects[1] = %+v, want %+v", params.Objects[1], objects[1])
	}
}

func TestParseParametersValidInputWithURLOverrides(t *testing.T) {
	objects := []ObjectEntry{
		{BwSecretID: "11111111-1111-1111-1111-111111111111", FileName: "db-password"},
	}

	attrs := buildAttributes(t, map[string]string{
		"organizationId": "22222222-2222-2222-2222-222222222222",
		"objects":        objectsJSON(t, objects),
		"apiUrl":         "https://api.bitwarden.eu",
		"identityUrl":    "https://identity.bitwarden.eu",
	})

	params, err := ParseParameters(attrs)
	if err != nil {
		t.Fatalf("ParseParameters returned unexpected error: %v", err)
	}

	if params.APIURL != "https://api.bitwarden.eu" {
		t.Errorf("APIURL = %q, want %q", params.APIURL, "https://api.bitwarden.eu")
	}

	if params.IdentityURL != "https://identity.bitwarden.eu" {
		t.Errorf("IdentityURL = %q, want %q", params.IdentityURL, "https://identity.bitwarden.eu")
	}
}

func TestParseParametersMissingOrganizationID(t *testing.T) {
	objects := []ObjectEntry{
		{BwSecretID: "11111111-1111-1111-1111-111111111111", FileName: "db-password"},
	}

	testCases := map[string]map[string]string{
		"absent": {
			"objects": objectsJSON(t, objects),
		},
		"empty": {
			"organizationId": "",
			"objects":        objectsJSON(t, objects),
		},
		"whitespace": {
			"organizationId": "   ",
			"objects":        objectsJSON(t, objects),
		},
	}

	for name, params := range testCases {
		t.Run(name, func(t *testing.T) {
			attrs := buildAttributes(t, params)

			_, err := ParseParameters(attrs)
			if err == nil {
				t.Fatal("ParseParameters unexpectedly succeeded")
			}

			if !strings.Contains(err.Error(), "organizationId") {
				t.Errorf("error = %q, want mention of organizationId", err.Error())
			}
		})
	}
}

func TestParseParametersMalformedObjects(t *testing.T) {
	validOrgID := "22222222-2222-2222-2222-222222222222"

	testCases := []struct {
		name       string
		objects    string
		wantErrSub string
	}{
		{
			name:       "absent",
			objects:    "",
			wantErrSub: "objects is required",
		},
		{
			name:       "not JSON",
			objects:    "not-json",
			wantErrSub: "not a valid JSON array",
		},
		{
			name:       "not an array",
			objects:    `{"bwSecretId": "11111111-1111-1111-1111-111111111111", "fileName": "foo"}`,
			wantErrSub: "not a valid JSON array",
		},
		{
			name:       "empty array",
			objects:    `[]`,
			wantErrSub: "at least one entry",
		},
		{
			name:       "entry missing identifier",
			objects:    `[{"fileName": "foo"}]`,
			wantErrSub: "exactly one of bwSecretId or secretName is required",
		},
		{
			name:       "entry with both identifiers",
			objects:    `[{"bwSecretId": "11111111-1111-1111-1111-111111111111", "secretName": "foo", "fileName": "foo"}]`,
			wantErrSub: "only one of bwSecretId or secretName",
		},
		{
			name:       "entry missing fileName",
			objects:    `[{"bwSecretId": "11111111-1111-1111-1111-111111111111"}]`,
			wantErrSub: "fileName is required",
		},
		{
			name:       "entry with invalid filePermission",
			objects:    `[{"bwSecretId": "11111111-1111-1111-1111-111111111111", "fileName": "foo", "filePermission": "not-octal"}]`,
			wantErrSub: "filePermission",
		},
		{
			name:       "entry with out-of-range filePermission",
			objects:    `[{"bwSecretId": "11111111-1111-1111-1111-111111111111", "fileName": "foo", "filePermission": "1000"}]`,
			wantErrSub: "filePermission",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attrs := buildAttributes(t, map[string]string{
				"organizationId": validOrgID,
				"objects":        tc.objects,
			})

			_, err := ParseParameters(attrs)
			if err == nil {
				t.Fatal("ParseParameters unexpectedly succeeded")
			}

			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErrSub)
			}
		})
	}
}

func TestParseParametersEmptyAttributes(t *testing.T) {
	if _, err := ParseParameters(""); err == nil {
		t.Fatal("ParseParameters unexpectedly succeeded on empty attributes")
	}
}

func TestParseParametersMalformedAttributesJSON(t *testing.T) {
	if _, err := ParseParameters("not-json"); err == nil {
		t.Fatal("ParseParameters unexpectedly succeeded on malformed JSON")
	}
}

func TestParseParametersInvalidURLOverride(t *testing.T) {
	objects := []ObjectEntry{
		{BwSecretID: "11111111-1111-1111-1111-111111111111", FileName: "db-password"},
	}

	testCases := map[string]string{
		"apiUrl":      "not-a-url",
		"identityUrl": "not-a-url",
	}

	for field, value := range testCases {
		t.Run(field, func(t *testing.T) {
			attrs := buildAttributes(t, map[string]string{
				"organizationId": "22222222-2222-2222-2222-222222222222",
				"objects":        objectsJSON(t, objects),
				field:            value,
			})

			_, err := ParseParameters(attrs)
			if err == nil {
				t.Fatal("ParseParameters unexpectedly succeeded")
			}

			if !strings.Contains(err.Error(), field) {
				t.Errorf("error = %q, want mention of %q", err.Error(), field)
			}
		})
	}
}
