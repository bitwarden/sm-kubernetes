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
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(operatorsv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	bwApiUrl, identApiUrl, statePath, refreshIntervalSeconds := GetSettings()
	bwClientFactory := controller.NewBitwardenClientFactory(bwApiUrl, identApiUrl)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "479cde60.bitwarden.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.BitwardenSecretReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		BitwardenClientFactory: bwClientFactory,
		StatePath:              statePath,
		RefreshIntervalSeconds: refreshIntervalSeconds,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BitwardenSecret")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func GetSettings() (string, string, string, int) {
	bwApiUrl := strings.TrimSpace(os.Getenv("BW_API_URL"))
	identApiUrl := strings.TrimSpace(os.Getenv("BW_IDENTITY_API_URL"))
	statePath := strings.TrimSpace(os.Getenv("BW_SECRETS_MANAGER_STATE_PATH"))
	refreshIntervalSecondsStr := strings.TrimSpace(os.Getenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL"))
	refreshIntervalSeconds := 300

	if refreshIntervalSecondsStr != "" {
		value, err := strconv.Atoi(refreshIntervalSecondsStr)

		if err != nil {
			setupLog.Error(err, fmt.Sprintf("Invalid refresh interval supplied: %s.  Defaulting to 300 seconds.", refreshIntervalSecondsStr))
		} else if value >= 180 {
			refreshIntervalSeconds = value
		} else {
			setupLog.Info(fmt.Sprintf("Refresh interval value is below the minimum allowed value of 180 seconds. Reverting to the default 300 seconds. Value supplied: %d", value))
		}
	}

	if bwApiUrl != "" {
		_, err := url.ParseRequestURI(bwApiUrl)

		if err != nil {
			setupLog.Error(err, fmt.Sprintf("Bitwarden API URL is not valid.  Value supplied: %s", bwApiUrl))
			panic(err)
		}

		u, err := url.Parse(bwApiUrl)

		if err != nil || u.Scheme == "" || u.Host == "" {
			setupLog.Error(err, fmt.Sprintf("Bitwarden API URL is not valid.  Value supplied: %s", bwApiUrl))
			panic(err)
		}
	}

	if identApiUrl != "" {
		_, err := url.ParseRequestURI(identApiUrl)

		if err != nil {
			setupLog.Error(err, fmt.Sprintf("Bitwarden Identity URL is not valid.  Value supplied: %s", identApiUrl))
			panic(err)
		}

		u, err := url.ParseRequestURI(identApiUrl)

		if err != nil || u.Scheme == "" || u.Host == "" {
			setupLog.Error(err, fmt.Sprintf("Bitwarden Identity URL is not valid.  Value supplied: %s", identApiUrl))
			panic(err)
		}
	}

	if bwApiUrl == "" {
		bwApiUrl = "https://api.bitwarden.com"
	}

	if identApiUrl == "" {
		identApiUrl = "https://identity.bitwarden.com"
	}

	if statePath == "" {
		statePath = "/var/bitwarden/state"
	}

	return bwApiUrl, identApiUrl, statePath, refreshIntervalSeconds
}
