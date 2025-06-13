// testutils/fixture.go
package testutils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
	OrgId           string
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

func NewTestFixture(t *testing.T, runner *EnvTestRunner) *TestFixture {
	gomega.RegisterFailHandler(ginkgo.Fail)
	f := &TestFixture{
		StatePath:       "bin",
		RefreshInterval: 300,
	}
	f.setup(t, runner)
	return f
}

func (f *TestFixture) WithMockK8sClient(t *testing.T, configureMocks func(client *mocks.MockClient, statusWriter *mocks.MockStatusWriter)) *TestFixture {
	mockCtrl := gomock.NewController(t)
	mockK8sClient := mocks.NewMockClient(mockCtrl)
	mockStatusWriter := mocks.NewMockStatusWriter(mockCtrl)
	mockK8sClient.EXPECT().Status().Return(mockStatusWriter).AnyTimes()
	configureMocks(mockK8sClient, mockStatusWriter)
	f.Reconciler.Client = mockK8sClient
	f.MockCtrl = mockCtrl
	return f
}

func (f *TestFixture) setup(t *testing.T, runner *EnvTestRunner) {
	f.OrgId = uuid.New().String()
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	f.Cfg = runner.Config
	f.K8sClient = runner.Client
	f.TestEnv = runner.Environment

	// Setup manager
	// k8sManager, err := ctrl.NewManager(f.Cfg, ctrl.Options{Scheme: scheme.Scheme})
	// gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Setup context
	f.Ctx, f.Cancel = context.WithCancel(context.TODO())

	// // Wait for cache sync
	// gomega.Eventually(func() bool {
	// 	//return k8sManager.GetCache().WaitForCacheSync(f.Ctx)
	// 	ginkgo.GinkgoWriter.Printf("Waiting for Cache sync check")
	// 	synced := k8sManager.GetCache().WaitForCacheSync(f.Ctx)
	// 	ginkgo.GinkgoWriter.Printf("Cache sync check: synced=%v, time=%v\n", synced)
	// 	return synced
	// }, 10*time.Second, 100*time.Millisecond).Should(gomega.BeTrue(), "manager cache failed to sync")

	// Initialize reconciler
	// f.Reconciler = controller.BitwardenSecretReconciler{
	// 	Client:                  k8sManager.GetClient(),
	// 	Scheme:                  k8sManager.GetScheme(),
	// 	RefreshIntervalSeconds:  f.RefreshInterval,
	// 	StatePath:               f.StatePath,
	// 	SetK8sSecretAnnotations: controller.SetK8sSecretAnnotations,
	// }

	f.Reconciler = controller.BitwardenSecretReconciler{
		Client:                  runner.Client,
		Scheme:                  runner.Manager.GetScheme(),
		RefreshIntervalSeconds:  f.RefreshInterval,
		StatePath:               f.StatePath,
		SetK8sSecretAnnotations: controller.SetK8sSecretAnnotations,
	}

	// Setup mocks (will be initialized per test)
	f.MockCtrl = gomock.NewController(t)
	f.MockFactory = mocks.NewMockBitwardenClientFactory(f.MockCtrl)
	f.MockClient = mocks.NewMockBitwardenClientInterface(f.MockCtrl)
	f.MockSecrets = mocks.NewMockSecretsInterface(f.MockCtrl)
	f.Reconciler.BitwardenClientFactory = f.MockFactory
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

func (f *TestFixture) CreateDefaultAuthSecret(namespace string) (*corev1.Secret, error) {
	return f.CreateAuthSecret(AuthSecretName, namespace, AuthSecretKey, AuthSecretValue)
}

func (f *TestFixture) CreateAuthSecret(name, namespace, key, value string) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
	err := f.K8sClient.Create(f.Ctx, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func (f *TestFixture) CreateDefaultBitwardenSecret(namespace string, secretMap []operatorsv1.SecretMap) (*operatorsv1.BitwardenSecret, error) {
	return f.CreateBitwardenSecret(BitwardenSecretName, namespace, string(f.OrgId), SynchronizedSecretName, AuthSecretName, AuthSecretKey, secretMap, true)
}

func (f *TestFixture) CreateBitwardenSecret(name, namespace, orgID, secretName, authSecretName, authSecretKey string, secretMap []operatorsv1.SecretMap, onlyMappedSecrets bool) (*operatorsv1.BitwardenSecret, error) {
	bwSecret := &operatorsv1.BitwardenSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: operatorsv1.BitwardenSecretSpec{
			AuthToken: operatorsv1.AuthToken{
				SecretName: authSecretName,
				SecretKey:  authSecretKey,
			},
			SecretName:        secretName,
			OrganizationId:    orgID,
			SecretMap:         secretMap,
			OnlyMappedSecrets: onlyMappedSecrets,
		},
	}
	err := f.K8sClient.Create(f.Ctx, bwSecret)
	if err != nil {
		return nil, err
	}
	return bwSecret, nil
}

func (f *TestFixture) CreateDefaultSynchronizedSecret(namespace string, data map[string][]byte) (*corev1.Secret, error) {
	return f.CreateSynchronizedSecret(SynchronizedSecretName, namespace, data)
}

func (f *TestFixture) CreateSynchronizedSecret(name string, namespace string, data map[string][]byte) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{},
			Annotations: map[string]string{
				controller.AnnotationSyncTime:  time.Now().UTC().Format(time.RFC3339),
				controller.AnnotationCustomMap: "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	err := f.K8sClient.Create(f.Ctx, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}
func (f *TestFixture) CreateNamespace() string {
	f.Namespace = fmt.Sprintf("bitwarden-ns-%s", uuid.NewString())
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: f.Namespace},
	}
	gomega.Expect(f.K8sClient.Create(f.Ctx, &ns)).Should(gomega.Succeed())
	return f.Namespace
}

// func (f *TestFixture) Teardown() {
// 	if f.Namespace != "" {
// 		ns := corev1.Namespace{}
// 		nsName := types.NamespacedName{Name: f.Namespace}
// 		if err := f.K8sClient.Get(f.Ctx, nsName, &ns); err == nil {
// 			gomega.Expect(f.K8sClient.Delete(f.Ctx, &ns)).Should(gomega.Succeed())
// 		}
// 	}
// 	f.Cancel()
// 	f.MockCtrl.Finish()
// 	gomega.Expect(f.TestEnv.Stop()).NotTo(gomega.HaveOccurred())
// }

// testutils/fixture.go
func (f *TestFixture) Teardown() {
	if f.Namespace != "" {
		// Create direct client to bypass cache
		directClient, err := client.New(f.Cfg, client.Options{Scheme: scheme.Scheme})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: f.Namespace}}
		err = directClient.Delete(f.Ctx, &ns)
		if err != nil && !errors.IsNotFound(err) {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
	}
	f.Cancel()
	f.MockCtrl.Finish()
}
