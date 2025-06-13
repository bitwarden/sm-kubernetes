package controller_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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

	sdk "github.com/bitwarden/sdk-go"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	//+kubebuilder:scaffold:imports
	mocks "github.com/bitwarden/sm-kubernetes/internal/controller/test/mocks"
)

/*
import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	// apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sdk "github.com/bitwarden/sdk-go"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	controller_test_mocks "github.com/bitwarden/sm-kubernetes/internal/controller/test/mocks"
	ctrl "sigs.k8s.io/controller-runtime"
	//+kubebuilder:scaffold:imports
)
*/

var _ = Describe("BitwardenSecretfixture.Reconciler", Ordered, func() {
	orgId := uuid.New()

	var (
		bwSecretsResponse sdk.SecretsSyncResponse
		secretMap         []operatorsv1.SecretMap
		namespace         string
		fixture           testutils.TestFixture
	)

	BeforeAll(func() {
		secretsData := []sdk.SecretResponse{}
		secretMap = []operatorsv1.SecretMap{}

		for secretCount := range testutils.ExpectedNumOfSecrets {
			identifier := sdk.SecretIdentifierResponse{
				ID:             uuid.NewString(),
				Key:            uuid.NewString(),
				OrganizationID: orgId.String(),
			}

			//build a map mapping each Identifier to an human readable name based on index
			secretMap = append(secretMap, operatorsv1.SecretMap{BwSecretId: identifier.ID, SecretKeyName: fmt.Sprintf("secret_%d_key", secretCount)})

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

		bwSecretsResponse = sdk.SecretsSyncResponse{
			HasChanges: true,
			Secrets:    secretsData,
		}
	})

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

	Describe("Reconcile", func() {
		It("should complete a successful sync", func() {
			fixture.SetupDefaultCtrlMocks(false, &bwSecretsResponse) // Use default &bwSecretsResponse

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(bwSecret).NotTo(BeNil())

			//fixture.CleanDefaultSynchronizedSecret(namespace)

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

			result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

			// Verify created secret
			createdTargetSecret := &corev1.Secret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())
			Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))
			Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))

			// Verify annotations
			Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
			Expect(createdTargetSecret.Annotations[controller.AnnotationCustomMap]).NotTo(BeEmpty())

			Eventually(func(g Gomega) {
				// Verify SuccessfulSync condition and LastSuccessfulSyncTime
				updatedBwSecret := &operatorsv1.BitwardenSecret{}
				g.Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
				condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
			}).Should(Succeed())
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
		It("should skip reconciliation when last sync is within refresh interval", func() {
			fixture.SetupDefaultCtrlMocks(false, nil)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(bwSecret).NotTo(BeNil())

			// Update status with LastSuccessfulSyncTime
			syncTime := time.Now().UTC()
			bwSecret.Status = operatorsv1.BitwardenSecretStatus{
				LastSuccessfulSyncTime: metav1.Time{Time: syncTime},
			}
			Expect(fixture.K8sClient.Status().Update(fixture.Ctx, bwSecret)).Should(Succeed())

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

			result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
		It("should handle a missing auth token secret", func() {
			fixture.SetupDefaultCtrlMocks(false, nil)

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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
		It("should skip sync when no changes from Bitwarden API", func() {
			// Override mocks to return no changes
			noChangesResponse := sdk.SecretsSyncResponse{
				HasChanges: false,
				Secrets:    []sdk.SecretResponse{},
			}

			fixture.SetupDefaultCtrlMocks(false, &noChangesResponse)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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
		It("should create a new Kubernetes secret", func() {
			fixture.SetupDefaultCtrlMocks(false, nil)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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

		It("should handle a controller reference failure", func() {
			fixture.SetupDefaultCtrlMocks(false, nil)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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
								OrganizationId: orgId.String(),
								SecretMap:      secretMap,
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

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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
								OrganizationId: orgId.String(),
								SecretMap:      secretMap,
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

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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

			// Mock SetK8sSecretAnnotations to fail by overriding the function
			originalSetK8sSecretAnnotations := controller.SetK8sSecretAnnotations
			controller.SetK8sSecretAnnotations = func(bwSecret *operatorsv1.BitwardenSecret, secret *corev1.Secret) error {
				return fmt.Errorf("annotation setting failed")
			}
			defer func() { controller.SetK8sSecretAnnotations = originalSetK8sSecretAnnotations }()

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testutils.BitwardenSecretName,
					Namespace: namespace,
				},
			}

			result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).NotTo(HaveOccurred()) // Annotation failure is logged but doesn't fail reconciliation
			Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

			// Verify SuccessfulSync condition (sync completes despite annotation failure)
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))

			// Verify secret was updated despite annotation failure
			updatedSecret := &corev1.Secret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, updatedSecret)).Should(Succeed())
			Expect(updatedSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			Expect(len(updatedSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))
			Expect(updatedSecret.Data).NotTo(HaveKey("old-key"))
		})
		It("should sync all secrets with partial mapping when OnlyMappedSecrets is false", func() {
			fixture.SetupDefaultCtrlMocks(false, nil)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			// Create partial SecretMap (map 5 out of 10 secrets)
			partialSecretMap := []operatorsv1.SecretMap{}
			for i := range 5 {
				partialSecretMap = append(partialSecretMap, secretMap[i])
			}

			bwSecret, err := fixture.CreateBitwardenSecret(testutils.BitwardenSecretName, namespace, fixture.OrgId, testutils.SynchronizedSecretName, testutils.AuthSecretName, testutils.AuthSecretKey, partialSecretMap, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(bwSecret).NotTo(BeNil())

			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}

			result, err := fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(fixture.Reconciler.RefreshIntervalSeconds) * time.Second))

			// Verify created secret
			createdTargetSecret := &corev1.Secret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())
			Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))
			Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets)) // All 10 secrets

			// Verify mapped secrets use SecretKeyName
			for _, mapping := range partialSecretMap {
				Expect(createdTargetSecret.Data).To(HaveKey(mapping.SecretKeyName))
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
					Expect(err).NotTo(HaveOccurred())
				}
			}
			Expect(unmappedCount).To(Equal(5)) // 5 unmapped secrets

			// Verify SuccessfulSync condition
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})
		It("should sync all secrets with Bitwarden IDs when OnlyMappedSecrets is false and no SecretMap", func() {
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

			// Verify BitwardenSecret status updates
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
		})
		It("should create empty secret when OnlyMappedSecrets is true and SecretMap is empty", func() {
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

			// Verify BitwardenSecret status
			updatedBwSecret := &operatorsv1.BitwardenSecret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
			condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
		})
		It("should handle non-existent secret IDs in SecretMap", func() {
			// Configure mocks with successful Bitwarden API response
			fixture.SetupDefaultCtrlMocks(false, nil)

			// Create SecretMap with one invalid (non-existent) secret ID
			invalidSecretMap := append(secretMap, operatorsv1.SecretMap{
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

			// Verify created Kubernetes secret
			createdTargetSecret := &corev1.Secret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())

			// Check secret metadata and type
			Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))

			// Verify only valid secrets are synced (expect original 10, not the invalid one)
			Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))
			for _, mapping := range secretMap {
				Expect(createdTargetSecret.Data).To(HaveKey(mapping.SecretKeyName))
			}
			Expect(createdTargetSecret.Data).NotTo(HaveKey("invalid_secret_key"))

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
		It("should handle invalid SecretMap entries", func() {
			// Configure mocks with successful Bitwarden API response
			fixture.SetupDefaultCtrlMocks(false, nil)

			// Create SecretMap with invalid entries (empty BwSecretId and SecretKeyName)
			invalidSecretMap := append(secretMap, operatorsv1.SecretMap{
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

			// Verify created Kubernetes secret
			createdTargetSecret := &corev1.Secret{}
			Expect(fixture.K8sClient.Get(fixture.Ctx, types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())

			// Check secret metadata and type
			Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
			Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))

			// Verify only valid secrets are synced (expect original 10, skipping invalid entries)
			Expect(len(createdTargetSecret.Data)).To(Equal(testutils.ExpectedNumOfSecrets))
			for _, mapping := range secretMap {
				Expect(createdTargetSecret.Data).To(HaveKey(mapping.SecretKeyName))
			}
			Expect(createdTargetSecret.Data).NotTo(HaveKey("empty_id_key"))
			Expect(createdTargetSecret.Data).NotTo(HaveKey(""))

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
		It("should handle large secret sets", func() {
			// Configure mocks with large Bitwarden API response
			largeNumOfSecrets := 1000
			largeSecretsData := []sdk.SecretResponse{}
			largeSecretMap := []operatorsv1.SecretMap{}
			for i := range largeNumOfSecrets {
				identifier := sdk.SecretIdentifierResponse{
					ID:             uuid.NewString(),
					Key:            uuid.NewString(),
					OrganizationID: orgId.String(),
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
					OrganizationID: orgId.String(),
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
		It("should handle status update conflicts", func() {
			// Configure mocks with successful Bitwarden API response
			fixture.SetupDefaultCtrlMocks(false, nil)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			bwSecret, err := fixture.CreateDefaultBitwardenSecret(namespace, secretMap)
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
})
