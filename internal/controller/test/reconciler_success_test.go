package controller_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	sdk "github.com/bitwarden/sdk-go/v2"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("BitwardenSecret Reconciler - Success Tests", Ordered, func() {
	var (
		namespace string
		fixture   testutils.TestFixture
	)

	BeforeEach(func() {
		fixture = *testutils.NewTestFixture(testContext, envTestRunner)
		namespace = fixture.CreateNamespace()
	})

	AfterAll(func() {
		fixture.Cancel()
	})

	AfterEach(func() {
		fixture.Teardown()
	})

	It("should complete a successful sync", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			// Verify created secret
			createdTargetSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())
			g.Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			g.Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))
			g.Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))

			// Verify annotations
			g.Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
			g.Expect(createdTargetSecret.Annotations[controller.AnnotationCustomMap]).NotTo(BeEmpty())

			// Verify SuccessfulSync condition and LastSuccessfulSyncTime
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
		}).Should(Succeed())
	})

	It("should skip reconciliation when last sync is within refresh interval", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		_, err = fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		// First reconcile: performs the actual sync and sets LastSuccessfulSyncTime
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		// Second reconcile: should skip because last sync is within refresh interval
		result, err = fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		fullInterval := time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", fullInterval))
	})

	It("should skip sync when no changes from Bitwarden API", func() {
		// Override mocks to return no changes
		noChangesResponse := sdk.SecretsSyncResponse{
			HasChanges: false,
			Secrets:    []sdk.SecretResponse{},
		}

		fixture.SetupDefaultCtrlMocks(false, &noChangesResponse)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			// Verify no SuccessfulSync condition (no sync occurred)
			createdSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, createdSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(createdSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).To(BeNil())
		})
	})
})
