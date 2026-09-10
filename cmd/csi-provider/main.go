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

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/bitwarden/sm-kubernetes/internal/csi"
)

// defaultEndpoint is the Unix Domain Socket path the Secrets Store CSI
// Driver expects provider gRPC servers to listen on.
const defaultEndpoint = "/etc/kubernetes/secrets-store-csi-providers/bitwarden.sock"

var setupLog = ctrl.Log.WithName("setup")

func main() {
	var endpoint string

	flag.StringVar(&endpoint, "endpoint", defaultEndpoint,
		"Path to the Unix Domain Socket the provider gRPC server listens on.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("starting bitwarden csi provider", "endpoint", endpoint, "version", csi.Version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := csi.Run(ctx, endpoint, ctrl.Log.WithName("csi-provider")); err != nil {
		setupLog.Error(err, "provider server exited with error")
		os.Exit(1)
	}
}
