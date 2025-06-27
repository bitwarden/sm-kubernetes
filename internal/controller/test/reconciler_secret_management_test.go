package controller_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("BitwardenSecret Reconciler - Secret Management Tests", Ordered, func() {
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

	It("should create a new Kubernetes secret", func() {
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
			g.Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets)) // From bwSecretsResponse

			// Verify SuccessfulSync condition
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	It("should update an existing Kubernetes secret", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Create existing target secret
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.SynchronizedSecretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"old-key": []byte("old-value"),
			},
		}
		Expect(fixture.K8sClient.Create(fixture.Ctx, existingSecret)).Should(Succeed())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			// Verify updated secret
			updatedSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, updatedSecret)).Should(Succeed())
			g.Expect(updatedSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			g.Expect(len(updatedSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets)) // Updated to bwSecretsResponse secrets
			g.Expect(updatedSecret.Data).NotTo(HaveKey("old-name"))                     // Old data replaced

			// Verify SuccessfulSync condition
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	It("should sync all secrets with partial mapping when OnlyMappedSecrets is false", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		// Create partial fixture.SecretMap (map 5 out of 10 secrets)
		partialSecretMap := []operatorsv1.SecretMap{}
		for i := range 5 {
			partialSecretMap = append(partialSecretMap, fixture.SecretMap[i])
		}

		bwSecret, err := fixture.CreateBitwardenSecret(testutils.BitwardenSecretName, namespace, fixture.OrgId, testutils.SynchronizedSecretName, testutils.AuthSecretName, testutils.AuthSecretKey, partialSecretMap, false)
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
			g.Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets)) // All 10 secrets

			// Verify mapped secrets use SecretKeyName
			for _, mapping := range partialSecretMap {
				g.Expect(createdTargetSecret.Data).To(HaveKey(mapping.SecretKeyName))
			}

			// Verify unmapped secrets use Bitwarden secret IDs
			unmappedCount := 0
			for key := range createdTargetSecret.Data {
				isMapped := false
				for _, mapping := range partialSecretMap {
					if key == mapping.SecretKeyName {
						isMapped = true
						break
					}
				}
				if !isMapped {
					unmappedCount++
					// Verify key is a valid UUID (Bitwarden secret ID)
					_, err := uuid.Parse(key)
					g.Expect(err).NotTo(HaveOccurred())
				}
			}
			g.Expect(unmappedCount).To(Equal(5)) // 5 unmapped secrets

			// Verify SuccessfulSync condition
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	It("should sync all secrets with Bitwarden IDs when OnlyMappedSecrets is false and no fixture.SecretMap", func() {
		// Configure mocks to return successful Bitwarden API response
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateBitwardenSecret(testutils.BitwardenSecretName, namespace, fixture.OrgId, testutils.SynchronizedSecretName, testutils.AuthSecretName, testutils.AuthSecretKey, nil, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Trigger reconciliation
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		// Verify reconciliation succeeded and requeue is set
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		// Verify created Kubernetes secret
		createdTargetSecret := &corev1.Secret{}
		Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())

		// Check secret metadata and type
		Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
		Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))

		// Verify all secrets are synced with Bitwarden secret IDs as keys
		Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))
		for key := range createdTargetSecret.Data {
			_, err := uuid.Parse(key)
			Expect(err).NotTo(HaveOccurred())
		}

		// Verify annotations
		Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
		Expect(createdTargetSecret.Annotations).NotTo(HaveKey(controller.AnnotationCustomMap))

		Eventually(func(g Gomega) {
			// Verify BitwardenSecret status updates
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
		})
	})

	It("should create empty secret when OnlyMappedSecrets is true and fixture.SecretMap is empty", func() {
		// Configure mocks with successful Bitwarden API response
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateBitwardenSecret(testutils.BitwardenSecretName, namespace, fixture.OrgId, testutils.SynchronizedSecretName, testutils.AuthSecretName, testutils.AuthSecretKey, nil, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Trigger reconciliation
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		// Verify reconciliation succeeded and requeue is set
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		// Verify created Kubernetes secret
		createdTargetSecret := &corev1.Secret{}
		Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())

		// Check secret metadata and type
		Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
		Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))

		// Verify secret has no data
		Expect(len(createdTargetSecret.Data)).To(Equal(0))

		// Verify annotations
		Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
		Expect(createdTargetSecret.Annotations).NotTo(HaveKey(controller.AnnotationCustomMap))

		Eventually(func(g Gomega) {
			// Verify BitwardenSecret status
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
		})
	})
})
