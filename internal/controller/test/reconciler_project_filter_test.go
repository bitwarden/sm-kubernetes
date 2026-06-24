package controller_test

import (
	"time"

	sdk "github.com/bitwarden/sdk-go/v2"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("BitwardenSecret Reconciler - Project ID Filter Tests", Ordered, func() {
	var (
		namespace       string
		fixture         testutils.TestFixture
		targetProjectId string
	)

	BeforeEach(func() {
		fixture = *testutils.NewTestFixture(testContext, envTestRunner)
		namespace = fixture.CreateNamespace()
		targetProjectId = uuid.NewString()
	})

	AfterAll(func() {
		fixture.Cancel()
	})

	AfterEach(func() {
		fixture.Teardown()
	})

	It("should sync only secrets belonging to the specified project ID", func() {
		otherProjectId := uuid.NewString()
		secrets := []sdk.SecretResponse{
			{ID: uuid.NewString(), Key: "secret_0", Value: "v0", OrganizationID: fixture.OrgId, ProjectID: &targetProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "secret_1", Value: "v1", OrganizationID: fixture.OrgId, ProjectID: &targetProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "secret_2", Value: "v2", OrganizationID: fixture.OrgId, ProjectID: &targetProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "other_0", Value: "o0", OrganizationID: fixture.OrgId, ProjectID: &otherProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "other_1", Value: "o1", OrganizationID: fixture.OrgId, ProjectID: &otherProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
		}
		fixture.SetupDefaultCtrlMocks(false, &sdk.SecretsSyncResponse{HasChanges: true, Secrets: secrets})

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret := &operatorsv1.BitwardenSecret{
			ObjectMeta: metav1.ObjectMeta{Name: testutils.BitwardenSecretName, Namespace: namespace},
			Spec: operatorsv1.BitwardenSecretSpec{
				AuthToken:         operatorsv1.AuthToken{SecretName: testutils.AuthSecretName, SecretKey: testutils.AuthSecretKey},
				SecretName:        testutils.SynchronizedSecretName,
				OrganizationId:    fixture.OrgId,
				OnlyMappedSecrets: false,
				ProjectId:         targetProjectId,
			},
		}
		Expect(fixture.K8sClient.Create(fixture.Ctx, bwSecret)).To(Succeed())

		Eventually(func(g Gomega) {
			fetched := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, fetched)).To(Succeed())
		}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			k8sSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, k8sSecret)).To(Succeed())
			// Only the 3 secrets from targetProjectId should be synced
			g.Expect(k8sSecret.Data).To(HaveLen(3))
		}).Should(Succeed())
	})

	It("should sync all secrets when no project ID is specified", func() {
		otherProjectId := uuid.NewString()
		secrets := []sdk.SecretResponse{
			{ID: uuid.NewString(), Key: "secret_0", Value: "v0", OrganizationID: fixture.OrgId, ProjectID: &targetProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "secret_1", Value: "v1", OrganizationID: fixture.OrgId, ProjectID: &targetProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "other_0", Value: "o0", OrganizationID: fixture.OrgId, ProjectID: &otherProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "other_1", Value: "o1", OrganizationID: fixture.OrgId, ProjectID: &otherProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
		}
		fixture.SetupDefaultCtrlMocks(false, &sdk.SecretsSyncResponse{HasChanges: true, Secrets: secrets})

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret := &operatorsv1.BitwardenSecret{
			ObjectMeta: metav1.ObjectMeta{Name: testutils.BitwardenSecretName, Namespace: namespace},
			Spec: operatorsv1.BitwardenSecretSpec{
				AuthToken:         operatorsv1.AuthToken{SecretName: testutils.AuthSecretName, SecretKey: testutils.AuthSecretKey},
				SecretName:        testutils.SynchronizedSecretName,
				OrganizationId:    fixture.OrgId,
				OnlyMappedSecrets: false,
				// No ProjectId: all secrets should be synced
			},
		}
		Expect(fixture.K8sClient.Create(fixture.Ctx, bwSecret)).To(Succeed())

		Eventually(func(g Gomega) {
			fetched := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, fetched)).To(Succeed())
		}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			k8sSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, k8sSecret)).To(Succeed())
			// All 4 secrets should be synced when no ProjectId filter is set
			g.Expect(k8sSecret.Data).To(HaveLen(4))
		}).Should(Succeed())
	})

	It("should create an empty K8s secret when no secrets match the specified project ID", func() {
		otherProjectId := uuid.NewString()
		nonMatchingProjectId := uuid.NewString()
		secrets := []sdk.SecretResponse{
			{ID: uuid.NewString(), Key: "secret_0", Value: "v0", OrganizationID: fixture.OrgId, ProjectID: &otherProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "secret_1", Value: "v1", OrganizationID: fixture.OrgId, ProjectID: &otherProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
		}
		fixture.SetupDefaultCtrlMocks(false, &sdk.SecretsSyncResponse{HasChanges: true, Secrets: secrets})

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret := &operatorsv1.BitwardenSecret{
			ObjectMeta: metav1.ObjectMeta{Name: testutils.BitwardenSecretName, Namespace: namespace},
			Spec: operatorsv1.BitwardenSecretSpec{
				AuthToken:         operatorsv1.AuthToken{SecretName: testutils.AuthSecretName, SecretKey: testutils.AuthSecretKey},
				SecretName:        testutils.SynchronizedSecretName,
				OrganizationId:    fixture.OrgId,
				OnlyMappedSecrets: false,
				ProjectId:         nonMatchingProjectId,
			},
		}
		Expect(fixture.K8sClient.Create(fixture.Ctx, bwSecret)).To(Succeed())

		Eventually(func(g Gomega) {
			fetched := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, fetched)).To(Succeed())
		}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			k8sSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, k8sSecret)).To(Succeed())
			// No secrets match the project filter, so the K8s secret should have no data
			g.Expect(k8sSecret.Data).To(BeEmpty())
		}).Should(Succeed())
	})

	It("should exclude secrets with a nil project ID when filtering by project ID", func() {
		secrets := []sdk.SecretResponse{
			{ID: uuid.NewString(), Key: "secret_0", Value: "v0", OrganizationID: fixture.OrgId, ProjectID: &targetProjectId, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "secret_1", Value: "v1", OrganizationID: fixture.OrgId, ProjectID: nil, CreationDate: time.Now(), RevisionDate: time.Now()},
			{ID: uuid.NewString(), Key: "secret_2", Value: "v2", OrganizationID: fixture.OrgId, ProjectID: nil, CreationDate: time.Now(), RevisionDate: time.Now()},
		}
		fixture.SetupDefaultCtrlMocks(false, &sdk.SecretsSyncResponse{HasChanges: true, Secrets: secrets})

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret := &operatorsv1.BitwardenSecret{
			ObjectMeta: metav1.ObjectMeta{Name: testutils.BitwardenSecretName, Namespace: namespace},
			Spec: operatorsv1.BitwardenSecretSpec{
				AuthToken:         operatorsv1.AuthToken{SecretName: testutils.AuthSecretName, SecretKey: testutils.AuthSecretKey},
				SecretName:        testutils.SynchronizedSecretName,
				OrganizationId:    fixture.OrgId,
				OnlyMappedSecrets: false,
				ProjectId:         targetProjectId,
			},
		}
		Expect(fixture.K8sClient.Create(fixture.Ctx, bwSecret)).To(Succeed())

		Eventually(func(g Gomega) {
			fetched := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, fetched)).To(Succeed())
		}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			k8sSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, k8sSecret)).To(Succeed())
			// Only the 1 secret with a matching ProjectID should be synced; nil-project secrets are excluded
			g.Expect(k8sSecret.Data).To(HaveLen(1))
		}).Should(Succeed())
	})
})
