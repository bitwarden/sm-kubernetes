package controller_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	sdk "github.com/bitwarden/sdk-go"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("BitwardenSecret Reconciler - Edge Case Tests", Ordered, func() {
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

	It("should handle non-existent secret IDs in SecretMap", func() {
		// Configure mocks with successful Bitwarden API response
		fixture.SetupDefaultCtrlMocks(false, nil)

		// Create fixture.SecretMap with one invalid (non-existent) secret ID
		invalidSecretMap := append(fixture.SecretMap, operatorsv1.SecretMap{
			BwSecretId:    uuid.NewString(), // Non-existent ID not in bwSecretsResponse
			SecretKeyName: "invalid_secret_key",
		})

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateBitwardenSecret(testutils.BitwardenSecretName, namespace, fixture.OrgId, testutils.SynchronizedSecretName, testutils.AuthSecretName, testutils.AuthSecretKey, invalidSecretMap, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Trigger reconciliation
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		// Verify reconciliation succeeded
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {

			// Verify created Kubernetes secret
			createdTargetSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())

			// Check secret metadata and type
			g.Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			g.Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))

			// Verify only valid secrets are synced (expect original 10, not the invalid one)
			g.Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))
			for _, mapping := range fixture.SecretMap {
				g.Expect(createdTargetSecret.Data).To(HaveKey(mapping.SecretKeyName))
			}
			g.Expect(createdTargetSecret.Data).NotTo(HaveKey("invalid_secret_key"))

			// Verify annotations
			g.Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
			g.Expect(createdTargetSecret.Annotations[controller.AnnotationCustomMap]).NotTo(BeEmpty())

			// Verify BitwardenSecret status
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
		})
	})

	It("should handle invalid SecretMap entries", func() {
		// Configure mocks with successful Bitwarden API response
		fixture.SetupDefaultCtrlMocks(false, nil)

		// Create fixture.SecretMap with invalid entries (empty BwSecretId and SecretKeyName)
		invalidSecretMap := append(fixture.SecretMap, operatorsv1.SecretMap{
			BwSecretId:    "", // Invalid: empty BwSecretId
			SecretKeyName: "empty_id_key",
		}, operatorsv1.SecretMap{
			BwSecretId:    uuid.NewString(), // Valid ID
			SecretKeyName: "",               // Invalid: empty SecretKeyName
		})

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateBitwardenSecret(testutils.BitwardenSecretName, namespace, fixture.OrgId, testutils.SynchronizedSecretName, testutils.AuthSecretName, testutils.AuthSecretKey, invalidSecretMap, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Trigger reconciliation
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		// Verify reconciliation succeeded
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {

			// Verify created Kubernetes secret
			createdTargetSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())

			// Check secret metadata and type
			g.Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			g.Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))

			// Verify only valid secrets are synced (expect original 10, skipping invalid entries)
			g.Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))
			for _, mapping := range fixture.SecretMap {
				g.Expect(createdTargetSecret.Data).To(HaveKey(mapping.SecretKeyName))
			}
			g.Expect(createdTargetSecret.Data).NotTo(HaveKey("empty_id_key"))
			g.Expect(createdTargetSecret.Data).NotTo(HaveKey(""))

			// Verify annotations
			g.Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
			g.Expect(createdTargetSecret.Annotations[controller.AnnotationCustomMap]).NotTo(BeEmpty())

			// Verify BitwardenSecret status
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
		})
	})

	It("should handle large secret sets", func() {
		// Configure mocks with large Bitwarden API response
		largeNumOfSecrets := 1000
		largeSecretsData := []sdk.SecretResponse{}
		largeSecretMap := []operatorsv1.SecretMap{}
		for i := range largeNumOfSecrets {
			identifier := sdk.SecretIdentifierResponse{
				ID:             uuid.NewString(),
				Key:            uuid.NewString(),
				OrganizationID: fixture.OrgId,
			}
			largeSecretMap = append(largeSecretMap, operatorsv1.SecretMap{
				BwSecretId:    identifier.ID,
				SecretKeyName: fmt.Sprintf("secret_%d_key", i),
			})
			projectId := uuid.NewString()
			largeSecretsData = append(largeSecretsData, sdk.SecretResponse{
				CreationDate:   time.Now().String(),
				ID:             identifier.ID,
				Key:            identifier.Key,
				Note:           uuid.NewString(),
				OrganizationID: fixture.OrgId,
				ProjectID:      &projectId,
				RevisionDate:   time.Now().String(),
				Value:          uuid.NewString(),
			})
		}
		largeSecretsResponse := sdk.SecretsSyncResponse{
			HasChanges: true,
			Secrets:    largeSecretsData,
		}

		fixture.SetupDefaultCtrlMocks(false, &largeSecretsResponse)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateBitwardenSecret(testutils.BitwardenSecretName, namespace, fixture.OrgId, testutils.SynchronizedSecretName, testutils.AuthSecretName, testutils.AuthSecretKey, largeSecretMap, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Trigger reconciliation
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		// Verify reconciliation succeeded
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		// Verify created Kubernetes secret
		createdTargetSecret := &corev1.Secret{}
		Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())

		// Check secret metadata and type
		Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
		Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))

		// Verify all secrets are synced
		Expect(len(createdTargetSecret.Data)).To(Equal(largeNumOfSecrets))
		for _, mapping := range largeSecretMap {
			Expect(createdTargetSecret.Data).To(HaveKey(mapping.SecretKeyName))
		}

		// Verify annotations
		Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
		Expect(createdTargetSecret.Annotations[controller.AnnotationCustomMap]).NotTo(BeEmpty())

		// Verify BitwardenSecret status
		updatedBwSecret := &operatorsv1.BitwardenSecret{}
		Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
		condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
	})
})
