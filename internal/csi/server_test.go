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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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

func TestServerMountStub(t *testing.T) {
	srv := NewServer(testLogger())

	resp, err := srv.Mount(context.Background(), &pb.MountRequest{
		Attributes: "{}",
		Secrets:    "{}",
		TargetPath: "/mnt/secrets",
		Permission: "420",
	})
	if err != nil {
		t.Fatalf("Mount returned error: %v", err)
	}

	if resp == nil {
		t.Fatal("Mount returned nil response")
	}

	if resp.GetError() != nil {
		t.Errorf("Mount returned unexpected Error: %v", resp.GetError())
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
