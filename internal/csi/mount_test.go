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
	"strings"
	"testing"

	sdk "github.com/bitwarden/sdk-go/v2"
)

func TestExtractAccessTokenValid(t *testing.T) {
	token, err := extractAccessToken(`{"token": "  my-token  "}`)
	if err != nil {
		t.Fatalf("extractAccessToken returned unexpected error: %v", err)
	}

	if token != "my-token" {
		t.Errorf("token = %q, want %q", token, "my-token")
	}
}

func TestExtractAccessTokenInvalid(t *testing.T) {
	testCases := map[string]struct {
		secretsJSON string
		wantErrSub  string
	}{
		"empty":            {"", "required"},
		"whitespace":       {"   ", "required"},
		"not JSON":         {"not-json", "failed to parse"},
		"missing key":      {`{"other": "value"}`, "token"},
		"empty token":      {`{"token": ""}`, "token"},
		"whitespace token": {`{"token": "   "}`, "token"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := extractAccessToken(tc.secretsJSON)
			if err == nil {
				t.Fatal("extractAccessToken unexpectedly succeeded")
			}

			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErrSub)
			}

			if strings.Contains(err.Error(), "my-token") {
				t.Errorf("error unexpectedly echoes back token content: %q", err.Error())
			}
		})
	}
}

func TestBuildMountFilesByID(t *testing.T) {
	byID := map[string]sdk.SecretResponse{
		"id-1": {ID: "id-1", Key: "db-password", Value: "s3cr3t"},
	}

	objects := []ObjectEntry{
		{BwSecretID: "id-1", FileName: "db-password"},
	}

	files, versions, err := buildMountFiles(objects, byID, nil)
	if err != nil {
		t.Fatalf("buildMountFiles returned unexpected error: %v", err)
	}

	if len(files) != 1 || len(versions) != 1 {
		t.Fatalf("len(files) = %d, len(versions) = %d, want 1, 1", len(files), len(versions))
	}

	if files[0].GetPath() != "db-password" {
		t.Errorf("Path = %q, want %q", files[0].GetPath(), "db-password")
	}

	if string(files[0].GetContents()) != "s3cr3t" {
		t.Errorf("Contents = %q, want %q", files[0].GetContents(), "s3cr3t")
	}

	if files[0].GetMode() != int32(maxFilePermission) {
		t.Errorf("Mode = %o, want default %o", files[0].GetMode(), maxFilePermission)
	}

	if versions[0].GetId() != "db-password" {
		t.Errorf("ObjectVersion.Id = %q, want %q", versions[0].GetId(), "db-password")
	}

	if versions[0].GetVersion() == "" {
		t.Error("ObjectVersion.Version is empty, want a stable content hash")
	}
}

func TestBuildMountFilesByName(t *testing.T) {
	byName := map[string][]sdk.SecretResponse{
		"api-key": {{ID: "id-2", Key: "api-key", Value: "abc123"}},
	}

	objects := []ObjectEntry{
		{SecretName: "api-key", FileName: "api-key.txt"},
	}

	files, _, err := buildMountFiles(objects, nil, byName)
	if err != nil {
		t.Fatalf("buildMountFiles returned unexpected error: %v", err)
	}

	if len(files) != 1 || string(files[0].GetContents()) != "abc123" {
		t.Fatalf("files = %+v, want a single file with contents %q", files, "abc123")
	}
}

func TestBuildMountFilesCustomFilePermission(t *testing.T) {
	byID := map[string]sdk.SecretResponse{
		"id-1": {ID: "id-1", Value: "s3cr3t"},
	}

	objects := []ObjectEntry{
		{BwSecretID: "id-1", FileName: "db-password", FilePermission: "0400"},
	}

	files, _, err := buildMountFiles(objects, byID, nil)
	if err != nil {
		t.Fatalf("buildMountFiles returned unexpected error: %v", err)
	}

	if files[0].GetMode() != 0o400 {
		t.Errorf("Mode = %o, want %o", files[0].GetMode(), 0o400)
	}
}

func TestBuildMountFilesByIDNotFound(t *testing.T) {
	objects := []ObjectEntry{
		{BwSecretID: "missing-id", FileName: "db-password"},
	}

	_, _, err := buildMountFiles(objects, map[string]sdk.SecretResponse{}, nil)
	if err == nil {
		t.Fatal("buildMountFiles unexpectedly succeeded for a missing secret id")
	}

	if !strings.Contains(err.Error(), "missing-id") {
		t.Errorf("error = %q, want mention of the missing id", err.Error())
	}
}

func TestBuildMountFilesByNameNotFound(t *testing.T) {
	objects := []ObjectEntry{
		{SecretName: "missing-name", FileName: "db-password"},
	}

	_, _, err := buildMountFiles(objects, nil, map[string][]sdk.SecretResponse{})
	if err == nil {
		t.Fatal("buildMountFiles unexpectedly succeeded for a missing secret name")
	}

	if !strings.Contains(err.Error(), "missing-name") {
		t.Errorf("error = %q, want mention of the missing name", err.Error())
	}
}

func TestBuildMountFilesByNameAmbiguous(t *testing.T) {
	byName := map[string][]sdk.SecretResponse{
		"dup": {
			{ID: "id-1", Key: "dup", Value: "v1"},
			{ID: "id-2", Key: "dup", Value: "v2"},
		},
	}

	objects := []ObjectEntry{
		{SecretName: "dup", FileName: "dup.txt"},
	}

	_, _, err := buildMountFiles(objects, nil, byName)
	if err == nil {
		t.Fatal("buildMountFiles unexpectedly succeeded for an ambiguous secret name")
	}

	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want mention of ambiguity", err.Error())
	}
}

// TestBuildMountFilesStableVersion verifies that ObjectVersion.Version is a
// pure function of secret content: unchanged content produces the same
// version across calls, and changed content produces a different version.
// This is what lets the driver's rotation reconcile skip republishing an
// unchanged secret.
func TestBuildMountFilesStableVersion(t *testing.T) {
	objects := []ObjectEntry{
		{BwSecretID: "id-1", FileName: "db-password"},
	}

	byIDUnchanged1 := map[string]sdk.SecretResponse{"id-1": {ID: "id-1", Value: "s3cr3t"}}
	byIDUnchanged2 := map[string]sdk.SecretResponse{"id-1": {ID: "id-1", Value: "s3cr3t"}}
	byIDChanged := map[string]sdk.SecretResponse{"id-1": {ID: "id-1", Value: "different"}}

	_, v1, err := buildMountFiles(objects, byIDUnchanged1, nil)
	if err != nil {
		t.Fatalf("buildMountFiles returned unexpected error: %v", err)
	}

	_, v2, err := buildMountFiles(objects, byIDUnchanged2, nil)
	if err != nil {
		t.Fatalf("buildMountFiles returned unexpected error: %v", err)
	}

	_, v3, err := buildMountFiles(objects, byIDChanged, nil)
	if err != nil {
		t.Fatalf("buildMountFiles returned unexpected error: %v", err)
	}

	if v1[0].GetVersion() != v2[0].GetVersion() {
		t.Errorf("version changed for unchanged content: %q != %q", v1[0].GetVersion(), v2[0].GetVersion())
	}

	if v1[0].GetVersion() == v3[0].GetVersion() {
		t.Errorf("version unchanged for different content: %q == %q", v1[0].GetVersion(), v3[0].GetVersion())
	}
}

func TestBuildMountFilesMultipleObjects(t *testing.T) {
	byID := map[string]sdk.SecretResponse{
		"id-1": {ID: "id-1", Value: "v1"},
		"id-2": {ID: "id-2", Value: "v2"},
	}

	objects := []ObjectEntry{
		{BwSecretID: "id-1", FileName: "file-1"},
		{BwSecretID: "id-2", FileName: "file-2"},
	}

	files, versions, err := buildMountFiles(objects, byID, nil)
	if err != nil {
		t.Fatalf("buildMountFiles returned unexpected error: %v", err)
	}

	if len(files) != 2 || len(versions) != 2 {
		t.Fatalf("len(files) = %d, len(versions) = %d, want 2, 2", len(files), len(versions))
	}
}
