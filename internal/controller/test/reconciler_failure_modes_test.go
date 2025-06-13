package controller_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/types"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	mocks "github.com/bitwarden/sm-kubernetes/internal/controller/test/mocks"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("BitwardenSecret Reconciler - Failure Mode Tests", Ordered, func() {
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

	It("should handle a controller reference failure", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Mock SetControllerReference to fail
		originalSetControllerReference := ctrl.SetControllerReference
		ctrl.SetControllerReference = func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
			return fmt.Errorf("controller reference failure")
		}
		defer func() { ctrl.SetControllerReference = originalSetControllerReference }()

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			// Verify no secret was created
			secret := &corev1.Secret{}
			err = fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, secret)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())

			// Verify FailedSync condition
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "FailedSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	It("should handle a secret creation failure", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		fixture.WithMockK8sClient(testContext, func(client *mocks.MockClient, statusWriter *mocks.MockStatusWriter) {
			// // Mock Get for BitwardenSecret
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj any, opts ...any) error {
					bw := obj.(*operatorsv1.BitwardenSecret)
					*bw = operatorsv1.BitwardenSecret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testutils.BitwardenSecretName, // "bw-secret"
							Namespace: namespace,
						},
						Spec: operatorsv1.BitwardenSecretSpec{
							AuthToken: operatorsv1.AuthToken{
								SecretName: testutils.AuthSecretName, // "bw-secret-sample-1"
								SecretKey:  testutils.AuthSecretKey,  // "token-key"
							},
							SecretName:     testutils.SynchronizedSecretName, // "bitwarden-k8s-secret-sample"
							OrganizationId: fixture.OrgId,
							SecretMap:      fixture.SecretMap,
						},
					}
					return nil
				}).
				AnyTimes()

			// Mock Get for auth secret
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.AuthSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj any, opts ...any) error {
					secret := obj.(*corev1.Secret)
					secret.ObjectMeta = metav1.ObjectMeta{
						Name:      testutils.AuthSecretName, // "bw-secret-sample-1"
						Namespace: namespace,
					}
					secret.Data = map[string][]byte{
						testutils.AuthSecretKey: []byte(testutils.AuthSecretValue), // "token-key": "abc-123"
					}
					return nil
				}).
				AnyTimes()

				// Mock Get for target secret (not found)
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj any, opts ...any) error {
					return errors.NewNotFound(schema.GroupResource{Group: "", Resource: "secrets"}, testutils.SynchronizedSecretName)
				}).
				AnyTimes()

			// Mock Create failure
			client.EXPECT().
				Create(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Secret{})).
				Return(fmt.Errorf("secret creation failed")).
				AnyTimes()

			statusWriter.EXPECT().
				Update(gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
				Return(nil).
				AnyTimes()
		})

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("secret creation failed"))
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))
	})

	It("should handle a secret patch failure", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Create existing Kubernetes secret
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

		fixture.WithMockK8sClient(testContext, func(client *mocks.MockClient, statusWriter *mocks.MockStatusWriter) {
			// Mock Get for BitwardenSecret
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj any, opts ...any) error {
					bw := obj.(*operatorsv1.BitwardenSecret)
					*bw = operatorsv1.BitwardenSecret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testutils.BitwardenSecretName, // "bw-secret"
							Namespace: namespace,
						},
						Spec: operatorsv1.BitwardenSecretSpec{
							AuthToken: operatorsv1.AuthToken{
								SecretName: testutils.AuthSecretName, // "bw-secret-sample-1"
								SecretKey:  testutils.AuthSecretKey,  // "token-key"
							},
							SecretName:     testutils.SynchronizedSecretName, // "bitwarden-k8s-secret-sample"
							OrganizationId: fixture.OrgId,
							SecretMap:      fixture.SecretMap,
						},
					}
					return nil
				}).
				AnyTimes()

			// Mock Get for auth secret
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.AuthSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj any, opts ...any) error {
					secret := obj.(*corev1.Secret)
					secret.ObjectMeta = metav1.ObjectMeta{
						Name:      testutils.AuthSecretName, // "bw-secret-sample-1"
						Namespace: namespace,
					}
					secret.Data = map[string][]byte{
						testutils.AuthSecretKey: []byte(testutils.AuthSecretValue), // "token-key": "abc-123"
					}
					return nil
				}).
				AnyTimes()

				// Mock Get for target secret (not found)
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj any, opts ...any) error {
					return nil
				}).
				AnyTimes()

			client.EXPECT().
				Patch(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Secret{}), gomock.Any()).
				Return(fmt.Errorf("secret patch failed")).
				AnyTimes()

			statusWriter.EXPECT().
				Update(gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
				Return(nil).
				AnyTimes()
		})

		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("secret patch failed"))
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))
	})

	It("should handle an annotation setting failure", func() {
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		// Create existing Kubernetes secret so we can prove it updates correctly
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

		// Mock SetK8sSecretAnnotations to fail by overriding the function
		originalSetK8sSecretAnnotations := fixture.Reconciler.SetK8sSecretAnnotations
		fixture.Reconciler.SetK8sSecretAnnotations = func(bwSecret *operatorsv1.BitwardenSecret, secret *corev1.Secret) error {
			return fmt.Errorf("annotation setting failed")
		}
		defer func() { fixture.Reconciler.SetK8sSecretAnnotations = originalSetK8sSecretAnnotations }()

		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      testutils.BitwardenSecretName,
				Namespace: namespace,
			},
		}

		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
		Expect(err).NotTo(HaveOccurred()) // Annotation failure is logged but doesn't fail reconciliation
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

		Eventually(func(g Gomega) {
			// Verify SuccessfulSync condition (sync completes despite annotation failure)
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			g.Expect(condition).NotTo(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))

			// Verify secret was updated despite annotation failure
			updatedSecret := &corev1.Secret{}
			g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, updatedSecret)).Should(Succeed())
			g.Expect(updatedSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			g.Expect(len(updatedSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))
			g.Expect(updatedSecret.Data).NotTo(HaveKey("old-key"))
		})
	})

	It("should handle status update conflicts", func() {
		// Configure mocks with successful Bitwarden API response
		fixture.SetupDefaultCtrlMocks(false, nil)

		_, err := fixture.CreateDefaultAuthSecret(namespace)
		Expect(err).NotTo(HaveOccurred())

		bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(bwSecret).NotTo(BeNil())

		conflictError := fmt.Errorf("conflict: Operation cannot be fulfilled on bitwardensecrets.k8s.bitwarden.com")

		fixture.WithMockK8sClient(testContext, func(client *mocks.MockClient, statusWriter *mocks.MockStatusWriter) {
			// Mock Get for BitwardenSecret
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{})).
				DoAndReturn(func(_ context.Context, key types.NamespacedName, obj any, _ ...any) error {
					bw := obj.(*operatorsv1.BitwardenSecret)
					*bw = *bwSecret
					return nil
				}).AnyTimes()

			// Mock Get for auth secret
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.AuthSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				DoAndReturn(func(_ context.Context, _ types.NamespacedName, obj any, _ ...any) error {
					secret := obj.(*corev1.Secret)
					secret.ObjectMeta = metav1.ObjectMeta{Name: testutils.AuthSecretName, Namespace: namespace}
					secret.Data = map[string][]byte{testutils.AuthSecretKey: []byte(testutils.AuthSecretValue)}
					return nil
				}).AnyTimes()

			// Mock Get for target secret (not found)
			client.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				Return(errors.NewNotFound(schema.GroupResource{Resource: "secrets"}, testutils.SynchronizedSecretName)).AnyTimes()

			// Mock Create for target secret
			client.EXPECT().
				Create(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Secret{})).
				Return(nil).AnyTimes()

			// Mock Patch for target secret
			client.EXPECT().
				Patch(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Secret{}), gomock.Any()).
				Return(nil).AnyTimes()

			// Mock Status().Update to simulate conflict
			statusWriter.EXPECT().
				Update(gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
				Return(conflictError).AnyTimes()
		})

		// Trigger reconciliation
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
		result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)

		// Verify reconciliation returns conflict error and requeues
		Expect(err).To(MatchError(conflictError))
		Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))
	})

})
