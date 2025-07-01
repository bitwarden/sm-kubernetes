package controller_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	mocks "github.com/bitwarden/sm-kubernetes/internal/controller/test/mocks"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("BitwardenSecret Reconciler - Error Handling Tests", Ordered, func() {
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

	It("should handle a non-existent BitwardenSecret", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      testutils.BitwardenSecretName,
				Namespace: namespace,
			},
		}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("\"%s\" not found", testutils.BitwardenSecretName)))
		Expect(result.Requeue).To(BeFalse())
	})

	It("should handle a generic error during BitwardenSecret retrieval", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)
		fixture.WithMockK8sClient(testContext, func(client *mocks.MockClient, statusWriter *mocks.MockStatusWriter) {
			// Configure mock to return a generic error for Get
			client.EXPECT().
				Get(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
				Return(errors.NewServiceUnavailable("API server error")).
				AnyTimes()

			statusWriter.EXPECT().
				Update(gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
				Return(nil).
				AnyTimes()
		})

		// Trigger reconciliation
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		// Expectations
		Expect(err).To(HaveOccurred())
		Expect(errors.IsNotFound(err)).To(BeFalse())
		Expect(err.Error()).To(ContainSubstring("API server error"))
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))
	})

	It("should handle a missing auth token secret", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))
	})

	It("should handle an invalid auth token secret key", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateAuthSecret(testutils.AuthSecretName, namespace, "Totally_Bogus_Key", "Totally Bogus Value")
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			// Verify FailedSync condition
			createdSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, createdSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(createdSecret.Status.Conditions, "FailedSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	It("should handle a Bitwarden API failure", func() {
		fixture.SetupDefaultCtrlMocks(true, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			// Verify FailedSync condition
			createdSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, createdSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(createdSecret.Status.Conditions, "FailedSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})
