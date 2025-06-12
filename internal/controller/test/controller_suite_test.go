package controller_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// Add necessary imports for your project
	//+kubebuilder:scaffold:imports
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
)

func TestBitwardenSecretsController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bitwarden Secrets Controller Suite")
}

func findProjectRoot() (string, error) {
	// Start from the current working directory or the directory of this file
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Traverse up until we find go.mod or reach the filesystem root
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root
			return "", fmt.Errorf("could not find project root: go.mod not found")
		}
		dir = parent
	}
}

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var statePath string
var refreshInterval int

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	refreshInterval = 300
	statePath = "bin"
	rootPath, _ := findProjectRoot()
	toolsPath := os.Getenv("KUBEBUILDER_ASSETS")

	if toolsPath == "" {

		k8sPath := filepath.Join(rootPath, "/bin/k8s")

		logf.Log.Info("Found rootPath and k8Path at:", rootPath, k8sPath)

		entries, err := os.ReadDir(k8sPath)
		if err != nil {
			logf.Log.Error(err, "Failed to read bin directory.  Make sure to run \"make test\" before debugging this test suite")
			panic(err)
		}

		for _, e := range entries {
			if e.IsDir() {
				os.Setenv("KUBEBUILDER_ASSETS", filepath.Join(k8sPath, e.Name()))
				break
			}
		}

		toolsPath = os.Getenv("KUBEBUILDER_ASSETS")

		if toolsPath == "" {
			err = fmt.Errorf("Failed to find envtest files under bin directory. Please run \"make test\" to resolve this issue.")
			panic(err)
		}
	}

	By("bootstrapping test environment")
	crdPath := filepath.Join(rootPath, "config/crd/bases")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{crdPath},
		//CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	apiServer := testEnv.ControlPlane.GetAPIServer()
	apiServer.Configure().Set("advertise-address", "127.0.0.1")

	var err error

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = operatorsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Clean all namespaces to ensure a fresh state
	protectedNamespaces := map[string]bool{
		"default":         true,
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}
	namespaceList := &corev1.NamespaceList{}
	err = k8sClient.List(context.Background(), namespaceList)
	Expect(err).NotTo(HaveOccurred())
	for _, ns := range namespaceList.Items {
		if protectedNamespaces[ns.Name] {
			continue // Skip protected namespaces
		}
		err = k8sClient.Delete(context.Background(), &ns)
		if err != nil && !errors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred(), "Failed to delete namespace %s", ns.Name)
		}
	}
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
