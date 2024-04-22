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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sdk "github.com/bitwarden/sdk/languages/go"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	controller_test_mocks "github.com/bitwarden/sm-kubernetes/internal/controller/test_mocks"
	ctrl "sigs.k8s.io/controller-runtime"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var namespace string
var statePath string
var refreshInterval int

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	refreshInterval = 300
	statePath = "bin"

	toolsPath := os.Getenv("KUBEBUILDER_ASSETS")

	if toolsPath == "" {
		entries, err := os.ReadDir("../../bin/k8s")
		if err != nil {
			logf.Log.Error(err, "Failed to read bin directory.  Make sure to run \"make test\" before debugging this test suite")
			panic(err)
		}

		for _, e := range entries {
			if e.IsDir() {
				os.Setenv("KUBEBUILDER_ASSETS", filepath.Join("../../bin/k8s", e.Name()))
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
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
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
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Bitwarden Secrets Controller", Ordered, func() {
	authSecretValue := "abc-123"
	orgId := uuid.New()
	authSecretName := "bw-secret-sample-1"
	authSecretKey := "token-key"
	name := "bw-secret"
	secretName := "bitwarden-k8s-secret-sample"
	count := 10
	timeout := time.Second * 10
	interval := time.Millisecond * 250

	var (
		t             GinkgoTestReporter
		mockCtrl      *gomock.Controller
		mockFactory   *controller_test_mocks.MockBitwardenClientFactory
		mockClient    *controller_test_mocks.MockBitwardenClientInterface
		mockSecrets   *controller_test_mocks.MockSecretsInterface
		ctx           context.Context
		cancel        context.CancelFunc
		bwSecrets     sdk.SecretsResponse
		bwSecretsList sdk.SecretIdentifiersResponse
		reconciler    BitwardenSecretReconciler
	)

	SetupDefaultCtrlMocks := func() {
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

		reconciler.BitwardenClientFactory = mockFactory
	}

	BeforeAll(func() {
		ctx, cancel = context.WithCancel(context.TODO())

		k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
		})
		Expect(err).ToNot(HaveOccurred())
		reconciler = BitwardenSecretReconciler{
			Client:                 k8sManager.GetClient(),
			Scheme:                 k8sManager.GetScheme(),
			BitwardenClientFactory: mockFactory,
			RefreshIntervalSeconds: refreshInterval,
			StatePath:              statePath,
		}
		reconciler.SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			err = k8sManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		listData := []sdk.SecretIdentifierResponse{}
		secretsData := []sdk.SecretResponse{}

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

		bwSecretsList = sdk.SecretIdentifiersResponse{
			Data: listData,
		}

		bwSecrets = sdk.SecretsResponse{
			Data: secretsData,
		}

	})

	BeforeEach(func() {
		mockCtrl = gomock.NewController(t)

		mockFactory = controller_test_mocks.NewMockBitwardenClientFactory(mockCtrl)
		mockClient = controller_test_mocks.NewMockBitwardenClientInterface(mockCtrl)
		mockSecrets = controller_test_mocks.NewMockSecretsInterface(mockCtrl)

		namespace = fmt.Sprintf("bitwarden-ns-%s", uuid.NewString())

		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

	})

	AfterAll(func() {
		cancel()
	})

	AfterEach(func() {
		nsName := types.NamespacedName{
			Namespace: namespace,
			Name:      namespace,
		}

		ns := corev1.Namespace{}
		Expect(k8sClient.Get(ctx, nsName, &ns)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &ns)).Should(Succeed())

		mockCtrl.Finish()
	})

	It("Creates a synchronized K8s secret without a mapping", func() {
		SetupDefaultCtrlMocks()

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

		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}
		Expect(k8sClient.Get(ctx, bwSecretName, &bwSecret)).Should(Succeed())
		year, month, day := time.Now().UTC().Date()
		hour := time.Now().UTC().Hour()
		minute := time.Now().UTC().Minute()

		Expect(k8sSecret.ObjectMeta.Name).Should(Equal(secretName))
		Expect(k8sSecret.ObjectMeta.Namespace).Should(Equal(namespace))
		Expect(len(k8sSecret.ObjectMeta.Labels)).Should(Equal(1))
		Expect(k8sSecret.ObjectMeta.Labels["k8s.bitwarden.com/bw-secret"]).Should(Equal(string(bwSecret.UID)))
		Expect(k8sSecret.ObjectMeta.OwnerReferences[0].UID).Should(Equal(bwSecret.UID))
		Expect(k8sSecret.Type).Should(Equal(corev1.SecretTypeOpaque))
		Eventually(func() bool {
			return len(k8sSecret.ObjectMeta.Annotations) == 1
		}, timeout, interval).Should(BeTrue())
		Expect(k8sSecret.ObjectMeta.Annotations["k8s.bitwarden.com/sync-time"]).Should(Satisfy(func(s string) bool {
			timeVar, err := time.Parse(time.RFC3339Nano, k8sSecret.ObjectMeta.Annotations["k8s.bitwarden.com/sync-time"])
			anYear, anMonth, anDay := timeVar.UTC().Date()

			return err == nil &&
				anYear == year &&
				anMonth == month &&
				anDay == day &&
				timeVar.UTC().Hour() == hour &&
				timeVar.UTC().Minute() == minute
		}))

		Expect(len(k8sSecret.Data)).Should(Equal(count))
		for i := 0; i < count; i++ {
			id := bwSecrets.Data[i].ID
			value := bwSecrets.Data[i].Value
			Expect(string(k8sSecret.Data[id])).Should(Equal(value))
		}

		statYear, statMonth, statDay := bwSecret.Status.LastSuccessfulSyncTime.UTC().Date()
		Expect(statYear).Should(Equal(year))
		Expect(statMonth).Should(Equal(month))
		Expect(statDay).Should(Equal(day))
		Expect(bwSecret.Status.LastSuccessfulSyncTime.UTC().Hour()).Should(Equal(hour))
		Expect(bwSecret.Status.LastSuccessfulSyncTime.UTC().Minute()).Should(Equal(minute))

		Expect(len(bwSecret.Status.Conditions)).Should(Equal(1))
		Expect(bwSecret.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
		Expect(bwSecret.Status.Conditions[0].Reason).Should(Equal("ReconciliationComplete"))
		Expect(bwSecret.Status.Conditions[0].Type).Should(Equal("SuccessfulSync"))
		Expect(bwSecret.Status.Conditions[0].Message).Should(Equal(fmt.Sprintf("Completed sync for %s/%s", namespace, name)))

		Expect(k8sClient.Delete(ctx, &bwSecret)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("Creates a synchronized K8s secret with a mapping", func() {
		SetupDefaultCtrlMocks()

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
		customMapping := []operatorsv1.SecretMap{ // Adding a map for the first 3 values
			{BwSecretId: bwSecrets.Data[0].ID, SecretKeyName: uuid.NewString()},
			{BwSecretId: bwSecrets.Data[1].ID, SecretKeyName: uuid.NewString()},
			{BwSecretId: bwSecrets.Data[2].ID, SecretKeyName: uuid.NewString()},
			{BwSecretId: uuid.NewString(), SecretKeyName: uuid.NewString()}, // Test to verify nothing will break if the source ID does not exist
		}

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
				SecretMap:      customMapping,
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
			return err == nil && len(k8sSecret.ObjectMeta.Annotations) > 0
		}, timeout, interval).Should(BeTrue())

		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, timeout, interval).Should(BeTrue())

		year, month, day := time.Now().UTC().Date()
		hour := time.Now().UTC().Hour()
		minute := time.Now().UTC().Minute()

		Expect(k8sSecret.ObjectMeta.Name).Should(Equal(secretName))
		Expect(k8sSecret.ObjectMeta.Namespace).Should(Equal(namespace))
		Expect(len(k8sSecret.ObjectMeta.Labels)).Should(Equal(1))
		Expect(k8sSecret.ObjectMeta.Labels["k8s.bitwarden.com/bw-secret"]).Should(Equal(string(bwSecret.UID)))
		Expect(k8sSecret.ObjectMeta.OwnerReferences[0].UID).Should(Equal(bwSecret.UID))
		Expect(k8sSecret.Type).Should(Equal(corev1.SecretTypeOpaque))
		Expect(k8sSecret.ObjectMeta.Annotations["k8s.bitwarden.com/sync-time"]).Should(Satisfy(func(s string) bool {
			timeVar, err := time.Parse(time.RFC3339Nano, k8sSecret.ObjectMeta.Annotations["k8s.bitwarden.com/sync-time"])
			anYear, anMonth, anDay := timeVar.UTC().Date()

			return err == nil &&
				anYear == year &&
				anMonth == month &&
				anDay == day &&
				timeVar.UTC().Hour() == hour &&
				timeVar.UTC().Minute() == minute
		}))

		Expect(k8sSecret.ObjectMeta.Annotations["k8s.bitwarden.com/custom-map"]).Should(Satisfy(func(s string) bool {
			anMap := []operatorsv1.SecretMap{}
			err := json.Unmarshal([]byte(s), &anMap)

			if err != nil {
				return false
			}

			for i := 0; i < len(customMapping); i++ {
				if anMap[i] != customMapping[i] {
					return false
				}
			}

			return true
		}))

		Expect(len(k8sSecret.Data)).Should(Equal(count))
		for i := 0; i < count; i++ {
			var id string
			if i < 3 {
				id = customMapping[i].SecretKeyName
			} else {
				id = bwSecrets.Data[i].ID
			}

			value := bwSecrets.Data[i].Value
			Expect(string(k8sSecret.Data[id])).Should(Equal(value))
		}

		statYear, statMonth, statDay := bwSecret.Status.LastSuccessfulSyncTime.UTC().Date()
		Expect(statYear).Should(Equal(year))
		Expect(statMonth).Should(Equal(month))
		Expect(statDay).Should(Equal(day))
		Expect(bwSecret.Status.LastSuccessfulSyncTime.UTC().Hour()).Should(Equal(hour))
		Expect(bwSecret.Status.LastSuccessfulSyncTime.UTC().Minute()).Should(Equal(minute))

		Expect(len(bwSecret.Status.Conditions)).Should(Equal(1))
		Expect(bwSecret.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
		Expect(bwSecret.Status.Conditions[0].Reason).Should(Equal("ReconciliationComplete"))
		Expect(bwSecret.Status.Conditions[0].Type).Should(Equal("SuccessfulSync"))
		Expect(bwSecret.Status.Conditions[0].Message).Should(Equal(fmt.Sprintf("Completed sync for %s/%s", namespace, name)))

		Expect(k8sClient.Delete(ctx, &bwSecret)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("Fails to create synchronized K8s secret without auth secret", func() {
		SetupDefaultCtrlMocks()

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
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, 3, interval).Should(BeFalse())

		year := time.Now().UTC().Year()
		statYear := bwSecret.Status.LastSuccessfulSyncTime.UTC().Year()
		Expect(statYear).ShouldNot(Equal(year))

		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, timeout, interval).Should(BeTrue())
		Expect(bwSecret.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(bwSecret.Status.Conditions[0].Reason).Should(Equal("ReconciliationFailed"))
		Expect(bwSecret.Status.Conditions[0].Type).Should(Equal("FailedSync"))
		Expect(bwSecret.Status.Conditions[0].Message).Should(Equal(fmt.Sprintf("Error pulling authorization token secret - Secret \"%s\" not found", authSecretName)))

		Expect(k8sClient.Delete(ctx, &bwSecret)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("Fails to create synchronized K8s secret with GetBitwardenClient failure", func() {
		testError := errors.NewBadRequest("Something bad happened.")
		apiUrl := "http://api.bitwarden.com"
		identityUrl := "http://identity.bitwarden.com"

		mockFactory.
			EXPECT().
			GetBitwardenClient().
			Return(nil, testError).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetApiUrl().
			Return(apiUrl).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetIdentityApiUrl().
			Return(identityUrl).
			AnyTimes()

		reconciler.BitwardenClientFactory = mockFactory

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
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, 3, interval).Should(BeFalse())

		year := time.Now().UTC().Year()
		statYear := bwSecret.Status.LastSuccessfulSyncTime.UTC().Year()
		Expect(statYear).ShouldNot(Equal(year))

		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, timeout, interval).Should(BeTrue())
		Expect(bwSecret.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(bwSecret.Status.Conditions[0].Reason).Should(Equal("ReconciliationFailed"))
		Expect(bwSecret.Status.Conditions[0].Type).Should(Equal("FailedSync"))
		Expect(bwSecret.Status.Conditions[0].Message).Should(Equal(fmt.Sprintf("Error pulling Secret Manager secrets from API => API: %s -- Identity: %s -- State: %s -- OrgId: %s  - %s", apiUrl, identityUrl, statePath, orgId, testError)))

		Expect(k8sClient.Delete(ctx, &bwSecret)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("Fails to create synchronized K8s secret with GetAccessToken failure", func() {
		testError := errors.NewBadRequest("Something bad happened.")
		apiUrl := "http://api.bitwarden.com"
		identityUrl := "http://identity.bitwarden.com"

		mockClient.
			EXPECT().
			AccessTokenLogin(gomock.Cond(func(x any) bool { return x.(string) == authSecretValue }), gomock.Eq(&statePath)).
			Return(testError).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetBitwardenClient().
			Return(mockClient, nil).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetApiUrl().
			Return(apiUrl).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetIdentityApiUrl().
			Return(identityUrl).
			AnyTimes()

		reconciler.BitwardenClientFactory = mockFactory

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
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, 3, interval).Should(BeFalse())

		year := time.Now().UTC().Year()
		statYear := bwSecret.Status.LastSuccessfulSyncTime.UTC().Year()
		Expect(statYear).ShouldNot(Equal(year))

		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, timeout, interval).Should(BeTrue())
		Expect(bwSecret.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(bwSecret.Status.Conditions[0].Reason).Should(Equal("ReconciliationFailed"))
		Expect(bwSecret.Status.Conditions[0].Type).Should(Equal("FailedSync"))
		Expect(bwSecret.Status.Conditions[0].Message).Should(Equal(fmt.Sprintf("Error pulling Secret Manager secrets from API => API: %s -- Identity: %s -- State: %s -- OrgId: %s  - %s", apiUrl, identityUrl, statePath, orgId, testError)))

		Expect(k8sClient.Delete(ctx, &bwSecret)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("Fails to create synchronized K8s secret with Secrets List failure", func() {
		testError := errors.NewBadRequest("Something bad happened.")
		apiUrl := "http://api.bitwarden.com"
		identityUrl := "http://identity.bitwarden.com"

		mockSecrets.
			EXPECT().
			List(gomock.Any()).
			Return(nil, testError).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetApiUrl().
			Return(apiUrl).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetIdentityApiUrl().
			Return(identityUrl).
			AnyTimes()

		SetupDefaultCtrlMocks()

		reconciler.BitwardenClientFactory = mockFactory

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
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, 3, interval).Should(BeFalse())

		year := time.Now().UTC().Year()
		statYear := bwSecret.Status.LastSuccessfulSyncTime.UTC().Year()
		Expect(statYear).ShouldNot(Equal(year))
		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, timeout, interval).Should(BeTrue())
		Expect(bwSecret.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(bwSecret.Status.Conditions[0].Reason).Should(Equal("ReconciliationFailed"))
		Expect(bwSecret.Status.Conditions[0].Type).Should(Equal("FailedSync"))
		Expect(bwSecret.Status.Conditions[0].Message).Should(Equal(fmt.Sprintf("Error pulling Secret Manager secrets from API => API: %s -- Identity: %s -- State: %s -- OrgId: %s  - %s", apiUrl, identityUrl, statePath, orgId, testError)))

		Expect(k8sClient.Delete(ctx, &bwSecret)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("Fails to create synchronized K8s secret with Secrets GetByIDs failure", func() {
		testError := errors.NewBadRequest("Something bad happened.")
		apiUrl := "http://api.bitwarden.com"
		identityUrl := "http://identity.bitwarden.com"

		mockSecrets.
			EXPECT().
			GetByIDS(gomock.Any()).
			Return(nil, testError).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetApiUrl().
			Return(apiUrl).
			AnyTimes()

		mockFactory.
			EXPECT().
			GetIdentityApiUrl().
			Return(identityUrl).
			AnyTimes()

		SetupDefaultCtrlMocks()

		reconciler.BitwardenClientFactory = mockFactory

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
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, 3, interval).Should(BeFalse())

		year := time.Now().UTC().Year()
		statYear := bwSecret.Status.LastSuccessfulSyncTime.UTC().Year()
		Expect(statYear).ShouldNot(Equal(year))

		bwSecretName := types.NamespacedName{Name: name, Namespace: namespace}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err == nil && len(bwSecret.Status.Conditions) > 0
		}, timeout, interval).Should(BeTrue())
		Expect(bwSecret.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
		Expect(bwSecret.Status.Conditions[0].Reason).Should(Equal("ReconciliationFailed"))
		Expect(bwSecret.Status.Conditions[0].Type).Should(Equal("FailedSync"))
		Expect(bwSecret.Status.Conditions[0].Message).Should(Equal(fmt.Sprintf("Error pulling Secret Manager secrets from API => API: %s -- Identity: %s -- State: %s -- OrgId: %s  - %s", apiUrl, identityUrl, statePath, orgId, testError)))

		Expect(k8sClient.Delete(ctx, &bwSecret)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bwSecretName, &bwSecret)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	It("Creates and requeues for the next round", func() {
		SetupDefaultCtrlMocks()

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

		s := runtime.NewScheme()

		if err := operatorsv1.AddToScheme(s); err != nil {
			t.Fatalf("Unable to add route scheme: (%v)", err)
		}

		objs := []runtime.Object{&bwSecret, &authSecret}

		cl := fake.NewClientBuilder().
			WithRuntimeObjects(objs...).
			Build()

		r := &BitwardenSecretReconciler{
			Client:                 cl,
			Scheme:                 s,
			BitwardenClientFactory: mockFactory,
			StatePath:              statePath,
			RefreshIntervalSeconds: refreshInterval,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}

		res, err := r.Reconcile(ctx, req)
		Expect(err).Should(BeNil())
		Expect(res.RequeueAfter).Should(Equal(time.Duration(300) * time.Second))

		k8sSecretName := types.NamespacedName{Name: secretName, Namespace: namespace}
		k8sSecret := &corev1.Secret{}
		err = r.Client.Get(ctx, k8sSecretName, k8sSecret)
		Expect(err).Should(BeNil())
		Expect(k8sSecret).ShouldNot(BeNil())
	})

	It("Returns without requeuing if BitwardenSecret deleted.", func() {
		SetupDefaultCtrlMocks()

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

		s := runtime.NewScheme()

		if err := operatorsv1.AddToScheme(s); err != nil {
			t.Fatalf("Unable to add route scheme: (%v)", err)
		}

		objs := []runtime.Object{&authSecret}

		cl := fake.NewClientBuilder().
			WithRuntimeObjects(objs...).
			Build()

		r := &BitwardenSecretReconciler{
			Client:                 cl,
			Scheme:                 s,
			BitwardenClientFactory: mockFactory,
			StatePath:              statePath,
			RefreshIntervalSeconds: refreshInterval,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}

		res, err := r.Reconcile(ctx, req)
		Expect(err).Should(BeNil())
		Expect(res.RequeueAfter).Should(Equal(time.Duration(0) * time.Second))

		k8sSecretName := types.NamespacedName{Name: secretName, Namespace: namespace}
		k8sSecret := &corev1.Secret{}
		err = r.Client.Get(ctx, k8sSecretName, k8sSecret)
		Expect(err).ShouldNot(BeNil())
	})

	It("Requeues if client throws error on BitwardenSecret lookup.", func() {
		SetupDefaultCtrlMocks()

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

		s := runtime.NewScheme()

		if err := operatorsv1.AddToScheme(s); err != nil {
			t.Fatalf("Unable to add route scheme: (%v)", err)
		}

		objs := []runtime.Object{&authSecret}

		cl := fake.NewClientBuilder().
			WithRuntimeObjects(objs...).
			Build()

		fcl := ErroringFakeClient{
			shouldErrorOnGet:    true,
			shouldErrorOnUpdate: false,
			shouldErrorOnCreate: false,
			shouldErrorOnDelete: false,
			Client:              cl,
			errorOnNames:        []types.NamespacedName{{Name: name, Namespace: namespace}},
		}

		r := &BitwardenSecretReconciler{
			Client:                 &fcl,
			Scheme:                 s,
			BitwardenClientFactory: mockFactory,
			StatePath:              statePath,
			RefreshIntervalSeconds: refreshInterval,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}

		res, err := r.Reconcile(ctx, req)
		Expect(err).ShouldNot(BeNil())
		Expect(res.RequeueAfter).Should(Equal(time.Duration(300) * time.Second))

		k8sSecretName := types.NamespacedName{Name: secretName, Namespace: namespace}
		k8sSecret := &corev1.Secret{}
		err = r.Client.Get(ctx, k8sSecretName, k8sSecret)
		Expect(err).ShouldNot(BeNil())
	})

	It("Requeues if it fails to create the secret", func() {
		SetupDefaultCtrlMocks()

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

		s := runtime.NewScheme()

		if err := operatorsv1.AddToScheme(s); err != nil {
			t.Fatalf("Unable to add route scheme: (%v)", err)
		}

		objs := []runtime.Object{&bwSecret, &authSecret}

		cl := fake.NewClientBuilder().
			WithRuntimeObjects(objs...).
			Build()

		fcl := ErroringFakeClient{
			shouldErrorOnGet:    false,
			shouldErrorOnUpdate: false,
			shouldErrorOnCreate: true,
			shouldErrorOnDelete: false,
			Client:              cl,
			errorOnNames:        []types.NamespacedName{{Name: secretName, Namespace: namespace}},
		}

		r := &BitwardenSecretReconciler{
			Client:                 &fcl,
			Scheme:                 s,
			BitwardenClientFactory: mockFactory,
			StatePath:              statePath,
			RefreshIntervalSeconds: refreshInterval,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}

		res, err := r.Reconcile(ctx, req)
		Expect(err).ShouldNot(BeNil())
		Expect(res.RequeueAfter).Should(Equal(time.Duration(300) * time.Second))

		k8sSecretName := types.NamespacedName{Name: secretName, Namespace: namespace}
		k8sSecret := &corev1.Secret{}
		err = r.Client.Get(ctx, k8sSecretName, k8sSecret)
		Expect(err).ShouldNot(BeNil())
	})

	It("Requeues if it fails to update the secret", func() {
		SetupDefaultCtrlMocks()

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

		s := runtime.NewScheme()

		if err := operatorsv1.AddToScheme(s); err != nil {
			t.Fatalf("Unable to add route scheme: (%v)", err)
		}

		objs := []runtime.Object{&bwSecret, &authSecret}

		cl := fake.NewClientBuilder().
			WithRuntimeObjects(objs...).
			Build()

		fcl := ErroringFakeClient{
			shouldErrorOnGet:    false,
			shouldErrorOnUpdate: true,
			shouldErrorOnCreate: false,
			shouldErrorOnDelete: false,
			Client:              cl,
			errorOnNames:        []types.NamespacedName{{Name: secretName, Namespace: namespace}},
		}

		r := &BitwardenSecretReconciler{
			Client:                 &fcl,
			Scheme:                 s,
			BitwardenClientFactory: mockFactory,
			StatePath:              statePath,
			RefreshIntervalSeconds: refreshInterval,
		}

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}

		res, err := r.Reconcile(ctx, req)
		Expect(err).ShouldNot(BeNil())
		Expect(res.RequeueAfter).Should(Equal(time.Duration(300) * time.Second))

		k8sSecretName := types.NamespacedName{Name: secretName, Namespace: namespace}
		k8sSecret := &corev1.Secret{}
		err = fcl.Get(ctx, k8sSecretName, k8sSecret)
		Expect(err).Should(BeNil())
		Expect(k8sSecret).ShouldNot(BeNil())
	})
})

var _ = Describe("Bitwarden Client Factory", func() {
	It("Creates a client with the correct settings", func() {
		api := "https://api.me"
		ident := "https://ident.me"
		expectedType, _ := sdk.NewBitwardenClient(&api, &ident)
		factory := NewBitwardenClientFactory(api, ident)
		client, err := factory.GetBitwardenClient()
		Expect(err).Should(BeNil())
		Expect(client).ShouldNot(BeNil())
		Expect(client).Should(BeAssignableToTypeOf(expectedType))
		Expect(factory.GetApiUrl()).Should(Equal(api))
		Expect(factory.GetIdentityApiUrl()).Should(Equal(ident))
	})
})

type GinkgoTestReporter struct{}

func (g GinkgoTestReporter) Errorf(format string, args ...interface{}) {
	Fail(fmt.Sprintf(format, args...))
}

func (g GinkgoTestReporter) Fatalf(format string, args ...interface{}) {
	Fail(fmt.Sprintf(format, args...))
}

type ErroringFakeClient struct {
	client.Client
	shouldErrorOnGet    bool
	shouldErrorOnUpdate bool
	shouldErrorOnCreate bool
	shouldErrorOnDelete bool
	errorOnNames        []types.NamespacedName
}

func (p *ErroringFakeClient) Get(
	ctx context.Context,
	key types.NamespacedName,
	acc client.Object,
	opts ...client.GetOption) error {

	if p.shouldErrorOnGet {
		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == key.Namespace && x.Name == key.Name {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error getting")
		}
	}
	return p.Client.Get(ctx, key, acc, opts...)
}

func (p *ErroringFakeClient) Update(
	ctx context.Context,
	acc client.Object,
	opts ...client.UpdateOption) error {
	if p.shouldErrorOnUpdate {
		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == acc.GetNamespace() && x.Name == acc.GetName() {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error updating")
		}
	}
	return p.Client.Update(ctx, acc, opts...)
}

func (p *ErroringFakeClient) Create(
	ctx context.Context,
	acc client.Object,
	opts ...client.CreateOption) error {
	if p.shouldErrorOnCreate {
		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == acc.GetNamespace() && x.Name == acc.GetName() {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error creating")
		}
	}
	return p.Client.Create(ctx, acc, opts...)
}

func (p *ErroringFakeClient) Delete(
	ctx context.Context,
	acc client.Object,
	opts ...client.DeleteOption) error {
	if p.shouldErrorOnDelete {

		nameToFail := false
		for _, x := range p.errorOnNames {
			if x.Namespace == acc.GetNamespace() && x.Name == acc.GetName() {
				nameToFail = true
				break
			}
		}

		if nameToFail {
			return fmt.Errorf("Error deleting")
		}
	}
	return p.Client.Delete(ctx, acc, opts...)
}
