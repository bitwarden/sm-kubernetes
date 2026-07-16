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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"

	pb "sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"
)

// socketMode is the permission bits applied to the Unix Domain Socket file
// after binding, so that access to this IPC channel does not depend on the
// process umask or the parent directory's permissions.
const socketMode = 0600

// gracefulShutdownTimeout bounds how long Run waits for in-flight RPCs to
// finish during a graceful shutdown before forcibly stopping the server.
const gracefulShutdownTimeout = 5 * time.Second

// Listen creates a Unix Domain Socket listener at socketPath. It ensures the
// parent directory exists and removes any stale socket file left behind by a
// previous run before binding. The resulting socket file is chmod'd to
// socketMode so access does not depend on the process umask.
func Listen(socketPath string, log logr.Logger) (net.Listener, error) {
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create socket directory %s: %w", dir, err)
	}

	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("refusing to remove existing non-socket file at %s", socketPath)
		}

		log.Info("removing stale socket file", "endpoint", socketPath)

		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("failed to remove stale socket %s: %w", socketPath, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat socket path %s: %w", socketPath, err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", socketPath, err)
	}

	if err := os.Chmod(socketPath, socketMode); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to set permissions on socket %s: %w", socketPath, err)
	}

	return listener, nil
}

// NewGRPCServer builds a *grpc.Server with srv registered as the
// CSIDriverProvider implementation.
func NewGRPCServer(srv pb.CSIDriverProviderServer) *grpc.Server {
	grpcServer := grpc.NewServer()
	pb.RegisterCSIDriverProviderServer(grpcServer, srv)

	return grpcServer
}

// Run starts the provider gRPC server listening on the Unix Domain Socket at
// socketPath and serves requests until ctx is cancelled or the server
// encounters an unrecoverable error. It blocks until one of those occurs.
func Run(ctx context.Context, socketPath string, log logr.Logger) error {
	listener, err := Listen(socketPath, log)
	if err != nil {
		return err
	}

	srv := NewServer(log)
	defer srv.Close()

	grpcServer := NewGRPCServer(srv)

	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down provider server", "endpoint", socketPath)

		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(gracefulShutdownTimeout):
			log.Info("graceful shutdown timed out, forcing stop", "endpoint", socketPath)
			grpcServer.Stop()
		}

		return nil
	case err := <-errCh:
		return err
	}
}
