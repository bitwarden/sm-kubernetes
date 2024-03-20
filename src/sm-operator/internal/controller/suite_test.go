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

package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	sdk "github.com/bitwarden/sdk/languages/go"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var namespace string
var mockCtrl gomock.Controller
var statePath string
var refreshInterval int

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	mockCtrl = *gomock.NewController(t)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	namespace = "bitwarden-ns"
	refreshInterval = 300
	statePath = "bin"

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

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
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Bitwarden Secrets Controller", func() {
	authSecretValue := "abc-123"
	orgId := uuid.New()
	authSecretName := "bw-secret-sample-1"
	authSecretKey := "token-key"
	name := "bw-secret"
	secretName := "bitwarden-k8s-secret-sample"
	var ctx context.Context
	var cancel context.CancelFunc
	mockFactory := NewMockBitwardenClientFactory(&mockCtrl)
	mockClient := NewMockBitwardenClientInterface(&mockCtrl)
	mockSecrets := NewMockSecretsInterface(&mockCtrl)
	timeout := time.Second * 10
	interval := time.Millisecond * 250

	BeforeEach(OncePerOrdered, func() {
		ctx, cancel = context.WithCancel(context.TODO())

		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

		k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
		})
		Expect(err).ToNot(HaveOccurred())

		err = (&BitwardenSecretReconciler{
			Client:                 k8sManager.GetClient(),
			Scheme:                 k8sManager.GetScheme(),
			BitwardenClientFactory: mockFactory,
			RefreshIntervalSeconds: refreshInterval,
			StatePath:              statePath,
		}).SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			err = k8sManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		authSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        authSecretName,
				Namespace:   namespace,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{authSecretKey: []byte(authSecretValue)},
		}

		Expect(k8sClient.Create(ctx, &authSecret)).Should(Succeed())
	})
	AfterEach(func() {
		cancel()
	})
	It("Creates a synchronized K8s secret without a mapping", func() {
		listData := []sdk.SecretIdentifierResponse{}
		secretsData := []sdk.SecretResponse{}
		count := 10
		for i := 0; i < count; i++ {
			identifier := sdk.SecretIdentifierResponse{
				ID:             uuid.NewString(),
				Key:            uuid.NewString(),
				OrganizationID: orgId.String(),
			}

			listData = append(listData, identifier)
			projectId := uuid.NewString()
			secretsData = append(secretsData, sdk.SecretResponse{
				CreationDate:   time.Now().String(),
				ID:             identifier.ID,
				Key:            identifier.Key,
				Note:           uuid.NewString(),
				OrganizationID: orgId.String(),
				ProjectID:      &projectId,
				RevisionDate:   time.Now().String(),
				Value:          uuid.NewString(),
			})
		}

		bwSecretsList := sdk.SecretIdentifiersResponse{
			Data: listData,
		}

		bwSecrets := sdk.SecretsResponse{
			Data: secretsData,
		}

		mockSecrets.
			EXPECT().
			List(gomock.Cond(func(x any) bool { return x.(string) == orgId.String() })).
			Return(&bwSecretsList, nil).
			AnyTimes()

		mockSecrets.
			EXPECT().
			GetByIDS(gomock.Cond(func(x any) bool {
				arr := x.([]string)
				match := len(arr) == count

				if match {
					for i := 0; i < count; i++ {
						found := false
						matchMe := arr[i]
						for j := 0; j < count; j++ {
							matchTo := bwSecretsList.Data[j]

							if matchMe == matchTo.ID {
								found = true
								break
							}
						}

						match = found
						if !match {
							break
						}
					}
				}
				return match
			})).
			Return(&bwSecrets, nil).
			AnyTimes()

		mockClient.
			EXPECT().
			AccessTokenLogin(gomock.Cond(func(x any) bool { return x.(string) == authSecretValue }), gomock.Eq(&statePath)).
			Return(nil).
			AnyTimes()

		mockClient.
			EXPECT().
			GetSecrets().
			Return(mockSecrets).
			AnyTimes()

		mockClient.
			EXPECT().
			Close().
			AnyTimes()

		mockFactory.
			EXPECT().
			GetBitwardenClient().
			Return(mockClient, nil).
			AnyTimes()

		bwSecret := operatorsv1.BitwardenSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"label-key": "label-value",
				},
			},
			Spec: operatorsv1.BitwardenSecretSpec{
				OrganizationId: orgId.String(),
				SecretName:     secretName,
				AuthToken: operatorsv1.AuthToken{
					SecretName: authSecretName,
					SecretKey:  authSecretKey,
				},
			},
		}

		Expect(k8sClient.Create(ctx, &bwSecret)).Should(Succeed())

		k8sSecretName := types.NamespacedName{Name: secretName, Namespace: namespace}
		k8sSecret := &corev1.Secret{}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, k8sSecretName, k8sSecret)
			return err == nil
		}, timeout, interval).Should(BeTrue())

		Expect(k8sSecret.ObjectMeta.Name).Should(Equal(secretName))
		Expect(k8sSecret.ObjectMeta.Namespace).Should(Equal(namespace))
		Expect(len(k8sSecret.ObjectMeta.Labels)).Should(Equal(1))
		Expect(k8sSecret.ObjectMeta.Labels["k8s.bitwarden.com/bw-secret"]).Should(Equal(string(bwSecret.UID)))
		Expect(len(k8sSecret.Data)).Should(Equal(count))

		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}

		Expect(k8sClient.Get(ctx, bwSecretName, &bwSecret)).Should(Succeed())

		// Expect(bwSecret.Status.LastSuccessfulSyncTime.Date()).Should(Equal(time.Now().UTC().Date))
	})
})
