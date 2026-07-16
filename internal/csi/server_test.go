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
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	sdk "github.com/bitwarden/sdk-go/v2"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"
)

func testLogger() logr.Logger {
	return zap.New(zap.UseDevMode(true))
}

// shortTempDir returns a freshly created temp directory suitable for hosting
// a Unix Domain Socket. Unlike t.TempDir(), it is not nested under a
// per-test path derived from the test name, keeping the resulting socket
// path comfortably under the platform's sun_path length limit (104-108
// bytes depending on OS).
func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "csi")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return dir
}

func TestServerVersion(t *testing.T) {
	srv := NewServer(testLogger())

	resp, err := srv.Version(context.Background(), &pb.VersionRequest{Version: "v1alpha1"})
	if err != nil {
		t.Fatalf("Version returned error: %v", err)
	}

	if resp.GetRuntimeName() != ProviderName {
		t.Errorf("RuntimeName = %q, want %q", resp.GetRuntimeName(), ProviderName)
	}

	if resp.GetVersion() != ProtocolVersion {
		t.Errorf("Version = %q, want %q", resp.GetVersion(), ProtocolVersion)
	}

	if resp.GetRuntimeVersion() != Version {
		t.Errorf("RuntimeVersion = %q, want %q", resp.GetRuntimeVersion(), Version)
	}
}

// TestServerMountInvalidAttributes verifies that Mount rejects a request
// whose SecretProviderClass parameters don't parse, without ever attempting
// to contact Bitwarden Secrets Manager.
func TestServerMountInvalidAttributes(t *testing.T) {
	srv := NewServer(testLogger())

	_, err := srv.Mount(context.Background(), &pb.MountRequest{
		Attributes: "{}",
		Secrets:    "{}",
		TargetPath: "/mnt/secrets",
		Permission: "420",
	})
	if err == nil {
		t.Fatal("Mount unexpectedly succeeded on empty attributes")
	}

	if !strings.Contains(err.Error(), "organizationId") {
		t.Errorf("error = %q, want mention of organizationId", err.Error())
	}
}

// TestServerMountMissingToken verifies that Mount rejects a request whose
// nodePublishSecretRef secret doesn't carry an access token, without ever
// attempting to contact Bitwarden Secrets Manager.
func TestServerMountMissingToken(t *testing.T) {
	srv := NewServer(testLogger())

	objects := objectsJSON(t, []ObjectEntry{
		{BwSecretID: "11111111-1111-1111-1111-111111111111", FileName: "db-password"},
	})

	_, err := srv.Mount(context.Background(), &pb.MountRequest{
		Attributes: buildAttributes(t, map[string]string{
			"organizationId": "22222222-2222-2222-2222-222222222222",
			"objects":        objects,
		}),
		Secrets:    "{}",
		TargetPath: "/mnt/secrets",
		Permission: "420",
	})
	if err == nil {
		t.Fatal("Mount unexpectedly succeeded with no access token in nodePublishSecretRef secret")
	}

	if !strings.Contains(err.Error(), secretRefTokenKey) {
		t.Errorf("error = %q, want mention of %q", err.Error(), secretRefTokenKey)
	}
}

// TestServerMountEndToEnd exercises the full Mount flow (parameter parsing,
// token extraction, cache/session lookup, Secrets Manager sync, and
// mapping/aliasing) against a fake Secrets Manager client, verifying the
// returned files and that repeated Mounts of unchanged secrets produce
// stable object versions.
func TestServerMountEndToEnd(t *testing.T) {
	secretID := "11111111-1111-1111-1111-111111111111"

	sdkSecrets := []sdk.SecretResponse{
		{ID: secretID, Key: "db-password", Value: "s3cr3t"},
	}

	factory := newFakeClientFactory(func(orgID string, lastSync *time.Time) (*sdk.SecretsSyncResponse, error) {
		return &sdk.SecretsSyncResponse{HasChanges: true, Secrets: sdkSecrets}, nil
	})

	srv := newServerWithFactory(testLogger(), factory.newFactory)

	mountReq := &pb.MountRequest{
		Attributes: buildAttributes(t, map[string]string{
			"organizationId": "22222222-2222-2222-2222-222222222222",
			"objects": objectsJSON(t, []ObjectEntry{
				{BwSecretID: secretID, FileName: "db-password"},
			}),
		}),
		Secrets:    buildAttributes(t, map[string]string{secretRefTokenKey: "test-access-token"}),
		TargetPath: "/mnt/secrets",
	}

	resp, err := srv.Mount(context.Background(), mountReq)
	if err != nil {
		t.Fatalf("Mount returned error: %v", err)
	}

	if len(resp.GetFiles()) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(resp.GetFiles()))
	}

	file := resp.GetFiles()[0]
	if file.GetPath() != "db-password" || string(file.GetContents()) != "s3cr3t" {
		t.Errorf("Files[0] = %+v, want path %q contents %q", file, "db-password", "s3cr3t")
	}

	if len(resp.GetObjectVersion()) != 1 {
		t.Fatalf("len(ObjectVersion) = %d, want 1", len(resp.GetObjectVersion()))
	}

	firstVersion := resp.GetObjectVersion()[0].GetVersion()

	// A second Mount of the same unchanged secret should produce the same
	// object version, so the driver's rotation reconcile is a no-op.
	resp2, err := srv.Mount(context.Background(), mountReq)
	if err != nil {
		t.Fatalf("second Mount returned error: %v", err)
	}

	if resp2.GetObjectVersion()[0].GetVersion() != firstVersion {
		t.Errorf("object version changed across unchanged Mounts: %q != %q", resp2.GetObjectVersion()[0].GetVersion(), firstVersion)
	}

	// Each sequential Mount polls Secrets Manager for changes (that's how
	// rotation detection works), but the underlying session/login is reused
	// rather than recreated.
	if factory.syncCalls() != 2 {
		t.Errorf("Secrets().Sync called %d times, want 2", factory.syncCalls())
	}

	if factory.creationCalls() != 1 {
		t.Errorf("client factory invoked %d times, want 1 (the logged-in session should be cached/reused)", factory.creationCalls())
	}
}

// TestListenRemovesStaleSocket verifies that Listen cleans up a stale socket
// file left over from a previous run instead of failing to bind.
func TestListenRemovesStaleSocket(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "bitwarden.sock")

	// Simulate a stale socket file from a previous, uncleanly-terminated run.
	stale, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to create stale socket: %v", err)
	}
	stale.Close()

	listener, err := Listen(socketPath, testLogger())
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	defer listener.Close()

	if listener.Addr().String() != socketPath {
		t.Errorf("listener address = %q, want %q", listener.Addr().String(), socketPath)
	}
}

// TestListenSetsRestrictiveSocketPermissions verifies that Listen locks down
// the socket file's permission bits instead of relying on the process umask.
func TestListenSetsRestrictiveSocketPermissions(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "bitwarden.sock")

	listener, err := Listen(socketPath, testLogger())
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	defer listener.Close()

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("failed to stat socket: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("socket permissions = %o, want %o", perm, 0600)
	}
}

// TestListenRefusesNonSocketFile verifies that Listen does not blindly
// remove an unexpected pre-existing file at socketPath.
func TestListenRefusesNonSocketFile(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "bitwarden.sock")

	if err := os.WriteFile(socketPath, []byte("not a socket"), 0600); err != nil {
		t.Fatalf("failed to create non-socket file: %v", err)
	}

	if _, err := Listen(socketPath, testLogger()); err == nil {
		t.Fatal("Listen unexpectedly succeeded against a non-socket file")
	}

	if _, err := os.Stat(socketPath); err != nil {
		t.Errorf("expected non-socket file to remain, but it was removed: %v", err)
	}
}

// TestRunServesOverUnixSocket exercises the full gRPC/UDS plumbing: it starts
// the provider server via Run, dials it over the Unix Domain Socket with a
// real gRPC client, and issues a Version RPC.
func TestRunServesOverUnixSocket(t *testing.T) {
	dir := shortTempDir(t)
	socketPath := filepath.Join(dir, "bitwarden.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(ctx, socketPath, testLogger())
	}()

	dialer := func(_ context.Context, addr string) (net.Conn, error) {
		return net.Dial("unix", addr)
	}

	var conn *grpc.ClientConn
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for {
		//nolint:staticcheck // grpc.DialContext is used deliberately for a synchronous test dial.
		conn, err = grpc.DialContext(ctx, socketPath,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(dialer),
			grpc.WithBlock(),
			grpc.WithTimeout(200*time.Millisecond),
		)
		if err == nil {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("failed to dial provider socket %s: %v", socketPath, err)
		}

		time.Sleep(20 * time.Millisecond)
	}
	defer conn.Close()

	client := pb.NewCSIDriverProviderClient(conn)

	resp, err := client.Version(context.Background(), &pb.VersionRequest{Version: "v1alpha1"})
	if err != nil {
		t.Fatalf("Version RPC failed: %v", err)
	}

	if resp.GetRuntimeName() != ProviderName {
		t.Errorf("RuntimeName = %q, want %q", resp.GetRuntimeName(), ProviderName)
	}

	cancel()

	select {
	case err := <-runErrCh:
		if err != nil {
			t.Errorf("Run returned error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not shut down within timeout")
	}
}
