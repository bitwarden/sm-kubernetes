// testutils/fixture.go
package testutils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/bitwarden/sdk-go"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	mocks "github.com/bitwarden/sm-kubernetes/internal/controller/test/mocks"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestFixture struct {
	Cfg             *rest.Config
	K8sClient       client.Client
	TestEnv         *envtest.Environment
	MockCtrl        *gomock.Controller
	MockFactory     *mocks.MockBitwardenClientFactory
	MockClient      *mocks.MockBitwardenClientInterface
	MockSecrets     *mocks.MockSecretsInterface
	Reconciler      controller.BitwardenSecretReconciler
	Ctx             context.Context
	Cancel          context.CancelFunc
	Namespace       string
	StatePath       string
	RefreshInterval int
}

func NewTestFixture(t *testing.T) *TestFixture {
	gomega.RegisterFailHandler(ginkgo.Fail)
	f := &TestFixture{
		StatePath:       "bin",
		RefreshInterval: 300,
	}
	f.setup(t)
	return f
}

func (f *TestFixture) setup(t *testing.T) {
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	// Find project root
	rootPath, err := findProjectRoot()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Set KUBEBUILDER_ASSETS
	toolsPath := os.Getenv("KUBEBUILDER_ASSETS")
	if toolsPath == "" {
		k8sPath := filepath.Join(rootPath, "bin/k8s")
		entries, err := os.ReadDir(k8sPath)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for _, e := range entries {
			if e.IsDir() {
				os.Setenv("KUBEBUILDER_ASSETS", filepath.Join(k8sPath, e.Name()))
				break
			}
		}
		toolsPath = os.Getenv("KUBEBUILDER_ASSETS")
		gomega.Expect(toolsPath).NotTo(gomega.BeEmpty(), "KUBEBUILDER_ASSETS not set")
	}

	// Setup envtest
	crdPath := filepath.Join(rootPath, "config/crd/bases")
	f.TestEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
	}
	f.TestEnv.ControlPlane.GetAPIServer().Configure().Set("advertise-address", "127.0.0.1")

	f.Cfg, err = f.TestEnv.Start()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(f.Cfg).NotTo(gomega.BeNil())

	// Setup scheme
	err = operatorsv1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Setup client
	f.K8sClient, err = client.New(f.Cfg, client.Options{Scheme: scheme.Scheme})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(f.K8sClient).NotTo(gomega.BeNil())

	// Setup manager
	k8sManager, err := ctrl.NewManager(f.Cfg, ctrl.Options{Scheme: scheme.Scheme})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Setup context
	f.Ctx, f.Cancel = context.WithCancel(context.TODO())

	// Initialize reconciler
	f.Reconciler = controller.BitwardenSecretReconciler{
		Client:                 k8sManager.GetClient(),
		Scheme:                 k8sManager.GetScheme(),
		RefreshIntervalSeconds: f.RefreshInterval,
		StatePath:              f.StatePath,
	}

	// Setup mocks (will be initialized per test)
	f.MockCtrl = gomock.NewController(t)
	f.MockFactory = mocks.NewMockBitwardenClientFactory(f.MockCtrl)
	f.MockClient = mocks.NewMockBitwardenClientInterface(f.MockCtrl)
	f.MockSecrets = mocks.NewMockSecretsInterface(f.MockCtrl)
	f.Reconciler.BitwardenClientFactory = f.MockFactory

	// Start manager
	go func() {
		defer ginkgo.GinkgoRecover()
		err = k8sManager.Start(f.Ctx)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()

	// Clean namespaces
	f.cleanNamespaces()
}

func (f *TestFixture) SetupDefaultCtrlMocks(failing bool, syncResponse *sdk.SecretsSyncResponse) {
	f.MockFactory.
		EXPECT().
		GetApiUrl().
		Return("http://api.bitwarden.com").
		AnyTimes()

	f.MockFactory.
		EXPECT().
		GetIdentityApiUrl().
		Return("http://identity.bitwarden.com").
		AnyTimes()

	response := syncResponse
	if response == nil {
		response = &sdk.SecretsSyncResponse{
			HasChanges: true,
			Secrets:    []sdk.SecretResponse{},
		}
	}

	if failing {
		f.MockSecrets.
			EXPECT().
			Sync(gomock.Any(), gomock.Any()).
			Return(nil, fmt.Errorf("bitwarden api error")).
			AnyTimes()
	} else {
		f.MockSecrets.
			EXPECT().
			Sync(gomock.Any(), gomock.Any()).
			Return(response, nil).
			AnyTimes()
	}

	f.MockClient.
		EXPECT().
		AccessTokenLogin(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	f.MockClient.
		EXPECT().
		Secrets().
		Return(f.MockSecrets).
		AnyTimes()

	f.MockClient.
		EXPECT().
		Close().
		AnyTimes()

	f.MockFactory.
		EXPECT().
		GetBitwardenClient().
		Return(f.MockClient, nil).
		AnyTimes()
}

func (f *TestFixture) CreateNamespace() string {
	f.Namespace = fmt.Sprintf("bitwarden-ns-%s", uuid.NewString())
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: f.Namespace},
	}
	gomega.Expect(f.K8sClient.Create(f.Ctx, &ns)).Should(gomega.Succeed())
	return f.Namespace
}

func (f *TestFixture) cleanNamespaces() {
	protectedNamespaces := map[string]bool{
		"default":         true,
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}
	namespaceList := &corev1.NamespaceList{}
	gomega.Expect(f.K8sClient.List(f.Ctx, namespaceList)).NotTo(gomega.HaveOccurred())
	for _, ns := range namespaceList.Items {
		if protectedNamespaces[ns.Name] {
			continue
		}
		err := f.K8sClient.Delete(f.Ctx, &ns)
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
	}
}

func (f *TestFixture) Teardown() {
	if f.Namespace != "" {
		ns := corev1.Namespace{}
		nsName := types.NamespacedName{Name: f.Namespace}
		if err := f.K8sClient.Get(f.Ctx, nsName, &ns); err == nil {
			gomega.Expect(f.K8sClient.Delete(f.Ctx, &ns)).Should(gomega.Succeed())
		}
	}
	f.Cancel()
	f.MockCtrl.Finish()
	gomega.Expect(f.TestEnv.Stop()).NotTo(gomega.HaveOccurred())
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
