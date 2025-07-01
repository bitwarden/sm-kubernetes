package testutils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
)

// EnvTestRunner manages a shared envtest environment for test suites.
type EnvTestRunner struct {
	Config      *rest.Config
	Client      client.Client
	Environment *envtest.Environment
	Manager     ctrl.Manager
	Cancel      context.CancelFunc
}

// NewEnvTestRunner initializes and starts an envtest environment.
// Call in BeforeSuite and ensure Stop is called in AfterSuite.
func NewEnvTestRunner() *EnvTestRunner {
	runner := &EnvTestRunner{}
	runner.setup()
	return runner
}

func (runner *EnvTestRunner) setup() {
	startTime := time.Now()
	defer func() {
		ginkgo.GinkgoWriter.Printf("EnvTestRunner setup completed in %v\n", time.Since(startTime))
	}()

	// Setup logger
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	fmt.Fprintf(ginkgo.GinkgoWriter, "Starting EnvTestRunner setup at %v\n", startTime)

	// Find project root for CRD paths
	rootPath, err := findProjectRoot()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ginkgo.GinkgoWriter.Printf("Found project root in %v\n", time.Since(startTime))

	// Set KUBEBUILDER_ASSETS if not already set
	toolsPath := os.Getenv("KUBEBUILDER_ASSETS")
	if toolsPath == "" {
		k8sPath := filepath.Join(rootPath, "bin/k8s")
		entries, err := os.ReadDir(k8sPath)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for _, e := range entries {
			if e.IsDir() {
				toolsPath = filepath.Join(k8sPath, e.Name())
				os.Setenv("KUBEBUILDER_ASSETS", toolsPath)
				break
			}
		}
		gomega.Expect(toolsPath).NotTo(gomega.BeEmpty(), "KUBEBUILDER_ASSETS not set")
	}
	ginkgo.GinkgoWriter.Printf("Set KUBEBUILDER_ASSETS to %s in %v\n", toolsPath, time.Since(startTime))

	// Setup envtest
	crdPath := filepath.Join(rootPath, "config/crd/bases")
	runner.Environment = &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
	}
	runner.Environment.ControlPlane.GetAPIServer().Configure().Set("advertise-address", "127.0.0.1")
	ginkgo.GinkgoWriter.Printf("Configured envtest in %v\n", time.Since(startTime))

	// Start envtest
	runner.Config, err = runner.Environment.Start()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(runner.Config).NotTo(gomega.BeNil())
	ginkgo.GinkgoWriter.Printf("Started envtest in %v\n", time.Since(startTime))

	// Setup scheme
	err = operatorsv1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ginkgo.GinkgoWriter.Printf("Added scheme in %v\n", time.Since(startTime))

	// Setup client
	runner.Client, err = client.New(runner.Config, client.Options{Scheme: scheme.Scheme})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(runner.Client).NotTo(gomega.BeNil())

	// Setup manager
	runner.Manager, err = ctrl.NewManager(runner.Config, ctrl.Options{Scheme: scheme.Scheme})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ginkgo.GinkgoWriter.Printf("Created manager in %v\n", time.Since(startTime))

	// Setup client from manager (ensures cache usage)
	ctx, cancel := context.WithCancel(context.TODO())
	runner.Cancel = cancel
	runner.Client = runner.Manager.GetClient()
	gomega.Expect(runner.Client).NotTo(gomega.BeNil())
	ginkgo.GinkgoWriter.Printf("Created client in %v\n", time.Since(startTime))

	// Start manager and wait for cache sync
	go func() {
		defer ginkgo.GinkgoRecover()
		err = runner.Manager.Start(ctx)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()
	ginkgo.GinkgoWriter.Printf("Started manager goroutine in %v\n", time.Since(startTime))

	// Wait for cache to sync
	gomega.Eventually(func() bool {
		synced := runner.Manager.GetCache().WaitForCacheSync(ctx)
		ginkgo.GinkgoWriter.Printf("Cache sync check: synced=%v, time=%v\n", synced, time.Since(startTime))
		return synced
	}, 20*time.Second, 100*time.Millisecond).Should(gomega.BeTrue(), "manager cache failed to sync")

}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find project root: go.mod not found")
		}
		dir = parent
	}
}

// Stop terminates the envtest environment.
// Call in AfterSuite.
func (runner *EnvTestRunner) Stop() {
	runner.Cancel()
	gexec.KillAndWait()
	gomega.Expect(runner.Environment.Stop()).NotTo(gomega.HaveOccurred())
}
