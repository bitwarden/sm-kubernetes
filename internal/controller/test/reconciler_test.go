package controller_test

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

var (
	namespace string
)

var _ = Describe("BitwardenSecretReconciler", Ordered, func() {
	authSecretValue := "abc-123"
	orgId := uuid.New()
	authSecretName := "bw-secret-sample-1"
	authSecretKey := "token-key"
	name := "bw-secret"
	secretName := "bitwarden-k8s-secret-sample"
	expectedNumOfSecrets := 10
	// timeout := time.Second * 10
	// interval := time.Millisecond * 250

	var (
		testReporter      GinkgoTestReporter
		mockCtrl          *gomock.Controller
		mockFactory       *controller_test_mocks.MockBitwardenClientFactory
		mockClient        *controller_test_mocks.MockBitwardenClientInterface
		mockSecrets       *controller_test_mocks.MockSecretsInterface
		ctx               context.Context
		cancel            context.CancelFunc
		bwSecretsResponse sdk.SecretsSyncResponse
		reconciler        controller.BitwardenSecretReconciler
		secretMap         []operatorsv1.SecretMap
	)

	// SetupDefaultCtrlMocks configures the mocked Bitwarden Client factory.
	// If failing is true, Sync returns an error. If syncResponse is nil, defaults to &bwSecretsResponse.
	SetupDefaultCtrlMocks := func(failing bool, syncResponse *sdk.SecretsSyncResponse) {
		mockFactory.
			EXPECT().
			GetApiUrl().
			Return("http://api.bitwarden.com").
			AnyTimes()

		mockFactory.
			EXPECT().
			GetIdentityApiUrl().
			Return("http://identity.bitwarden.com").
			AnyTimes()

		// Default to &bwSecretsResponse if syncResponse is nil
		response := syncResponse
		if response == nil {
			response = &bwSecretsResponse
		}

		if failing {
			mockSecrets.
				EXPECT().
				Sync(gomock.Cond(func(x any) bool { return x.(string) == orgId.String() }), gomock.Any()).
				Return(nil, fmt.Errorf("Bitwarden API error")).
				AnyTimes()
		} else {
			mockSecrets.
				EXPECT().
				Sync(gomock.Cond(func(x any) bool { return x.(string) == orgId.String() }), gomock.Any()).
				Return(response, nil).
				AnyTimes()
		}

		mockClient.
			EXPECT().
			AccessTokenLogin(gomock.Cond(func(x any) bool { return x.(string) == authSecretValue }), gomock.Eq(&statePath)).
			Return(nil).
			AnyTimes()

		mockClient.
			EXPECT().
			Secrets().
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

		log.Default().Print(k8sManager.GetConfig().Host)
		Expect(err).ToNot(HaveOccurred())

		reconciler = controller.BitwardenSecretReconciler{
			Client:                 k8sManager.GetClient(),
			Scheme:                 k8sManager.GetScheme(),
			BitwardenClientFactory: mockFactory,
			RefreshIntervalSeconds: refreshInterval,
			StatePath:              statePath,
		}

		_ = controller.BitwardenSecretReconciler{
			Client:                 k8sManager.GetClient(),
			Scheme:                 k8sManager.GetScheme(),
			BitwardenClientFactory: mockFactory,
			RefreshIntervalSeconds: refreshInterval,
			StatePath:              statePath,
		}

		reconciler.SetupWithManager(k8sManager)

		Expect(err).ToNot(HaveOccurred())

		// spawns a parallel routine to start the k8sManager
		go func() {
			defer GinkgoRecover()
			err = k8sManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		secretsData := []sdk.SecretResponse{}
		secretMap = []operatorsv1.SecretMap{}

		for secretCount := range expectedNumOfSecrets {
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
		mockCtrl = gomock.NewController(testReporter)

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

	Describe("Reconcile", func() {
		/*
			It("should handle a non-existent BitwardenSecret", func() {
				SetupDefaultCtrlMocks(false, nil)

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("\"%s\" not found", name)))
				Expect(result.Requeue).To(BeFalse())
			})
			It("should handle a generic error during BitwardenSecret retrieval", func() {
				SetupDefaultCtrlMocks(false, nil)

				// Create a mock k8s client and status writer
				mockK8sClient := controller_test_mocks.NewMockClient(mockCtrl)
				mockStatusWriter := controller_test_mocks.NewMockStatusWriter(mockCtrl)

				// Configure mock to return a generic error for Get
				mockK8sClient.EXPECT().
					Get(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
					Return(errors.NewServiceUnavailable("API server error")).
					AnyTimes()

				// Configure mock for Status and Update (called by LogError)
				mockK8sClient.EXPECT().
					Status().
					Return(mockStatusWriter).
					AnyTimes()

				mockStatusWriter.EXPECT().
					Update(gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
					Return(nil).
					AnyTimes()

				// Temporarily set the reconciler's client to the mock
				originalClient := reconciler.Client
				reconciler.Client = mockK8sClient
				defer func() { reconciler.Client = originalClient }()

				// Trigger reconciliation
				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)

				// Expectations
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeFalse())
				Expect(err.Error()).To(ContainSubstring("API server error"))
				Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))
			})
					It("should skip reconciliation when last sync is within refresh interval", func() {
						SetupDefaultCtrlMocks(false, nil)

						// Create auth secret
						authSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      authSecretName,
								Namespace: namespace,
							},
							Data: map[string][]byte{
								authSecretKey: []byte(authSecretValue),
							},
						}
						Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

						// Create BitwardenSecret
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
								SecretName:     secretName,
								OrganizationId: orgId.String(),
							},
						}
						Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

						// Update status with LastSuccessfulSyncTime
						syncTime := time.Now().UTC()
						bwSecret.Status = operatorsv1.BitwardenSecretStatus{
							LastSuccessfulSyncTime: metav1.Time{Time: syncTime},
						}
						Expect(k8sClient.Status().Update(ctx, bwSecret)).Should(Succeed())

						// Verify status was persisted
						createdSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())
						Expect(createdSecret.Status.LastSuccessfulSyncTime.Time).To(BeTemporally("~", syncTime, time.Second), "LastSuccessfulSyncTime should be set")

						req := reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      name,
								Namespace: namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).NotTo(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))
					})
					It("should handle a missing auth token secret", func() {
						SetupDefaultCtrlMocks(false, nil)

						// Create BitwardenSecret
						bwSecret := &operatorsv1.BitwardenSecret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      name,
								Namespace: namespace,
							},
							Spec: operatorsv1.BitwardenSecretSpec{
								AuthToken: operatorsv1.AuthToken{
									SecretName: authSecretName + "fake",
									SecretKey:  authSecretKey,
								},
								SecretName:     secretName,
								OrganizationId: orgId.String(),
							},
						}
						Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

						// Verify secret was created
						createdSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

						req := reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      name,
								Namespace: namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))
					})
					It("should handle an invalid auth token secret key", func() {
						SetupDefaultCtrlMocks(false, nil)

						// Create auth secret with invalid key
						authSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      authSecretName,
								Namespace: namespace,
							},
							Data: map[string][]byte{
								"wrong-key": []byte(authSecretValue), // authSecretKey="token-key" not present
							},
						}
						Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

						// Create BitwardenSecret
						bwSecret := &operatorsv1.BitwardenSecret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      name,
								Namespace: namespace,
							},
							Spec: operatorsv1.BitwardenSecretSpec{
								AuthToken: operatorsv1.AuthToken{
									SecretName: authSecretName,
									SecretKey:  authSecretKey, // Expects "token-key"
								},
								SecretName:     secretName,
								OrganizationId: orgId.String(),
							},
						}
						Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

						// Verify secret was created
						createdSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

						req := reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      name,
								Namespace: namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

						// Verify FailedSync condition
						createdSecret = &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())
						condition := apimeta.FindStatusCondition(createdSecret.Status.Conditions, "FailedSync")
						Expect(condition).NotTo(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					})

					It("should handle a Bitwarden API failure", func() {
						SetupDefaultCtrlMocks(true, nil)

						// Create auth secret
						authSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      authSecretName,
								Namespace: namespace,
							},
							Data: map[string][]byte{
								authSecretKey: []byte(authSecretValue),
							},
						}
						Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

						// Create BitwardenSecret
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
								SecretName:     secretName,
								OrganizationId: orgId.String(),
							},
						}
						Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

						// Verify secret was created
						createdSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

						req := reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      name,
								Namespace: namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

						// Verify FailedSync condition
						createdSecret = &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())
						condition := apimeta.FindStatusCondition(createdSecret.Status.Conditions, "FailedSync")
						Expect(condition).NotTo(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					})
					It("should skip sync when no changes from Bitwarden API", func() {
						// Override mocks to return no changes
						noChangesResponse := sdk.SecretsSyncResponse{
							HasChanges: false,
							Secrets:    []sdk.SecretResponse{},
						}

						SetupDefaultCtrlMocks(false, &noChangesResponse)

						// Create auth secret
						authSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      authSecretName,
								Namespace: namespace,
							},
							Data: map[string][]byte{
								authSecretKey: []byte(authSecretValue),
							},
						}
						Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

						// Create BitwardenSecret
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
								SecretName:     secretName,
								OrganizationId: orgId.String(),
							},
						}
						Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

						// Verify secret was created
						createdSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

						req := reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      name,
								Namespace: namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

						// Verify no SuccessfulSync condition (no sync occurred)
						createdSecret = &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())
						condition := apimeta.FindStatusCondition(createdSecret.Status.Conditions, "SuccessfulSync")
						Expect(condition).To(BeNil())

					})
					It("should create a new Kubernetes secret", func() {
						SetupDefaultCtrlMocks(false, nil) // Use default &bwSecretsResponse

						// Create auth secret
						authSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      authSecretName,
								Namespace: namespace,
							},
							Data: map[string][]byte{
								authSecretKey: []byte(authSecretValue),
							},
						}
						Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

						// Create BitwardenSecret
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
								SecretName:     secretName,
								OrganizationId: orgId.String(),
								SecretMap:      secretMap,
							},
						}
						Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

						// Verify secret was created
						createdSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

						targetSecret := &corev1.Secret{}

						// Clean up target secret if it exists
						err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, targetSecret)
						if err == nil {
							Expect(k8sClient.Delete(ctx, targetSecret)).Should(Succeed())
						} else if !errors.IsNotFound(err) {
							Fail(fmt.Sprintf("Failed to check target secret: %v", err))
						}

						req := reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      name,
								Namespace: namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

						// Verify created secret
						createdTargetSecret := &corev1.Secret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())
						Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
						Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))
						Expect(len(createdTargetSecret.Data)).To(Equal(expectedNumOfSecrets)) // From bwSecretsResponse

						// Verify SuccessfulSync condition
						updatedBwSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
						condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
						Expect(condition).NotTo(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					})
					It("should update an existing Kubernetes secret", func() {
						SetupDefaultCtrlMocks(false, nil) // Use default &bwSecretsResponse

						// Create auth secret
						authSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      authSecretName,
								Namespace: namespace,
							},
							Data: map[string][]byte{
								authSecretKey: []byte(authSecretValue),
							},
						}
						Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

						// Create BitwardenSecret
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
								SecretName:     secretName,
								OrganizationId: orgId.String(),
								SecretMap:      secretMap,
							},
						}
						Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

						// Verify secret was created
						createdSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

						// Create existing target secret
						existingSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      secretName,
								Namespace: namespace,
							},
							Data: map[string][]byte{
								"old-key": []byte("old-value"),
							},
						}
						Expect(k8sClient.Create(ctx, existingSecret)).Should(Succeed())

						req := reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      name,
								Namespace: namespace,
							},
						}

						result, err := reconciler.Reconcile(ctx, req)
						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

						// Verify updated secret
						updatedSecret := &corev1.Secret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, updatedSecret)).Should(Succeed())
						Expect(updatedSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
						Expect(len(updatedSecret.Data)).To(Equal(expectedNumOfSecrets)) // Updated to bwSecretsResponse secrets
						Expect(updatedSecret.Data).NotTo(HaveKey("old-name"))           // Old data replaced

						// Verify SuccessfulSync condition
						updatedBwSecret := &operatorsv1.BitwardenSecret{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
						condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
						Expect(condition).NotTo(BeNil())
						Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					})
				It("should handle a controller reference failure", func() {
					SetupDefaultCtrlMocks(false, nil) // Use default &bwSecretsResponse

					// Create auth secret
					authSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      authSecretName,
							Namespace: namespace,
						},
						Data: map[string][]byte{
							authSecretKey: []byte(authSecretValue),
						},
					}
					Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

					// Create BitwardenSecret
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
							SecretName:     secretName,
							OrganizationId: orgId.String(),
						},
					}
					Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

					// Verify secret was created
					createdSecret := &operatorsv1.BitwardenSecret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

					// Clean up target secret if it exists
					targetSecret := &corev1.Secret{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, targetSecret)
					if err == nil {
						Expect(k8sClient.Delete(ctx, targetSecret)).Should(Succeed())
					} else if !errors.IsNotFound(err) {
						Fail(fmt.Sprintf("Failed to check target secret: %v", err))
					}

					// Mock SetControllerReference to fail
					originalSetControllerReference := ctrl.SetControllerReference
					ctrl.SetControllerReference = func(owner, controlled metav1.Object, scheme *runtime.Scheme) error {
						return fmt.Errorf("controller reference failure")
					}
					defer func() { ctrl.SetControllerReference = originalSetControllerReference }()

					req := reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      name,
							Namespace: namespace,
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).To(HaveOccurred())
					Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

					// Verify no secret was created
					secret := &corev1.Secret{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
					Expect(errors.IsNotFound(err)).To(BeTrue())

					// Verify FailedSync condition
					updatedBwSecret := &operatorsv1.BitwardenSecret{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
					condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "FailedSync")
					Expect(condition).NotTo(BeNil())
					Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				})
		*/
		It("should handle a secret creation failure", func() {
			SetupDefaultCtrlMocks(false, nil) // Use default &bwSecretsResponse

			// Create auth secret
			authSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      authSecretName, // "bw-secret-sample-1"
					Namespace: namespace,
				},
				Data: map[string][]byte{
					authSecretKey: []byte(authSecretValue),
				},
			}
			Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

			// Create BitwardenSecret
			bwSecret := &operatorsv1.BitwardenSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name, // "bw-secret"
					Namespace: namespace,
				},
				Spec: operatorsv1.BitwardenSecretSpec{
					AuthToken: operatorsv1.AuthToken{
						SecretName: authSecretName,
						SecretKey:  authSecretKey,
					},
					SecretName:     secretName, // "bitwarden-k8s-secret-sample"
					OrganizationId: orgId.String(),
					SecretMap:      secretMap,
				},
			}
			Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

			// Verify BitwardenSecret was created
			createdSecret := &operatorsv1.BitwardenSecret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

			// Mock k8sClient
			mockK8sClient := controller_test_mocks.NewMockClient(mockCtrl)
			mockStatusWriter := controller_test_mocks.NewMockStatusWriter(mockCtrl)

			// // Mock Get for BitwardenSecret
			mockK8sClient.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: name, Namespace: namespace}), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...any) error {
					bw := obj.(*operatorsv1.BitwardenSecret)
					*bw = operatorsv1.BitwardenSecret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name, // "bw-secret"
							Namespace: namespace,
						},
						Spec: operatorsv1.BitwardenSecretSpec{
							AuthToken: operatorsv1.AuthToken{
								SecretName: authSecretName, // "bw-secret-sample-1"
								SecretKey:  authSecretKey,  // "token-key"
							},
							SecretName:     secretName, // "bitwarden-k8s-secret-sample"
							OrganizationId: orgId.String(),
							SecretMap:      secretMap,
						},
					}
					return nil
				}).
				AnyTimes()

			// Mock Get for auth secret
			mockK8sClient.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: authSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...any) error {
					secret := obj.(*corev1.Secret)
					secret.ObjectMeta = metav1.ObjectMeta{
						Name:      authSecretName, // "bw-secret-sample-1"
						Namespace: namespace,
					}
					secret.Data = map[string][]byte{
						authSecretKey: []byte(authSecretValue), // "token-key": "abc-123"
					}
					return nil
				}).
				AnyTimes()

			// // Mock Get for testing the test
			// mockK8sClient.EXPECT().
			// 	Get(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&corev1.Secret{})).
			// 	DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...any) error {
			// 		return nil
			// 	}).
			// 	AnyTimes()

			// Mock Get for target secret (not found)
			mockK8sClient.EXPECT().
				Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: secretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
				DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...any) error {
					return errors.NewNotFound(schema.GroupResource{Group: "", Resource: "secrets"}, secretName)
				}).
				AnyTimes()

			// Mock Create failure
			mockK8sClient.EXPECT().
				Create(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Secret{})).
				Return(fmt.Errorf("secret creation failed")).
				AnyTimes()

			// Mock Status().Update
			mockK8sClient.EXPECT().
				Status().
				Return(mockStatusWriter).
				AnyTimes()

			mockStatusWriter.EXPECT().
				Update(gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
				Return(nil).
				AnyTimes()

			// Temporarily set the reconciler's client to the mock
			originalClient := reconciler.Client
			reconciler.Client = mockK8sClient
			defer func() { reconciler.Client = originalClient }()

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      name, // "bw-secret"
					Namespace: namespace,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret creation failed"))
			Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))
		})
		/*
			It("should handle a secret patch failure", func() {
				SetupDefaultCtrlMocks(false, nil) // Use default &bwSecretsResponse

				// Create auth secret
				authSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      authSecretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						authSecretKey: []byte(authSecretValue),
					},
				}
				Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

				// Create BitwardenSecret
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
						SecretName:     secretName,
						OrganizationId: orgId.String(),
						SecretMap:      secretMap,
					},
				}
				Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

				// Create existing Kubernetes secret
				existingSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						"old-key": []byte("old-value"),
					},
				}
				Expect(k8sClient.Create(ctx, existingSecret)).Should(Succeed())

				// Mock k8sClient.Patch to fail
				mockK8sClient := controller_test_mocks.NewMockClient(mockCtrl)
				mockStatusWriter := controller_test_mocks.NewMockStatusWriter(mockCtrl)

				mockK8sClient.EXPECT().
					Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: authSecretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
					Return(nil).
					AnyTimes()

				mockK8sClient.EXPECT().
					Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: secretName, Namespace: namespace}), gomock.AssignableToTypeOf(&corev1.Secret{})).
					Return(nil).
					AnyTimes()

				mockK8sClient.EXPECT().
					Patch(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Secret{}), gomock.Any()).
					Return(fmt.Errorf("secret patch failed")).
					AnyTimes()

				mockK8sClient.EXPECT().
					Status().
					Return(mockStatusWriter).
					AnyTimes()

				mockStatusWriter.EXPECT().
					Update(gomock.Any(), gomock.AssignableToTypeOf(&operatorsv1.BitwardenSecret{}), gomock.Any()).
					Return(nil).
					AnyTimes()

				// Temporarily set the reconciler's client to the mock
				originalClient := reconciler.Client
				reconciler.Client = mockK8sClient
				defer func() { reconciler.Client = originalClient }()

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("secret patch failed"))
				Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

				// Verify FailedSync condition
				updatedBwSecret := &operatorsv1.BitwardenSecret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
				condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "FailedSync")
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Message).To(ContainSubstring("secret patch failed"))

				// Verify secret was not updated
				updatedSecret := &corev1.Secret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, updatedSecret)).Should(Succeed())
				Expect(updatedSecret.Data).To(HaveKeyWithValue("old-key", []byte("old-value")))
			})

			It("should handle an annotation setting failure", func() {
				SetupDefaultCtrlMocks(false, nil) // Use default &bwSecretsResponse

				// Create auth secret
				authSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      authSecretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						authSecretKey: []byte(authSecretValue),
					},
				}
				Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

				// Create BitwardenSecret
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
						SecretName:     secretName,
						OrganizationId: orgId.String(),
						SecretMap:      secretMap,
					},
				}
				Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

				// Create existing Kubernetes secret
				existingSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						"old-key": []byte("old-value"),
					},
				}
				Expect(k8sClient.Create(ctx, existingSecret)).Should(Succeed())

				// Mock SetK8sSecretAnnotations to fail by overriding the function
				originalSetK8sSecretAnnotations := controller.SetK8sSecretAnnotations
				controller.SetK8sSecretAnnotations = func(bwSecret *operatorsv1.BitwardenSecret, secret *corev1.Secret) error {
					return fmt.Errorf("annotation setting failed")
				}
				defer func() { controller.SetK8sSecretAnnotations = originalSetK8sSecretAnnotations }()

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred()) // Annotation failure is logged but doesn't fail reconciliation
				Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

				// Verify SuccessfulSync condition (sync completes despite annotation failure)
				updatedBwSecret := &operatorsv1.BitwardenSecret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
				condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))

				// Verify secret was updated despite annotation failure
				updatedSecret := &corev1.Secret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, updatedSecret)).Should(Succeed())
				Expect(updatedSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
				Expect(len(updatedSecret.Data)).To(Equal(expectedNumOfSecrets))
				Expect(updatedSecret.Data).NotTo(HaveKey("old-key"))
			})

			It("should complete a successful sync", func() {
				SetupDefaultCtrlMocks(false, nil) // Use default &bwSecretsResponse

				// Create auth secret
				authSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      authSecretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						authSecretKey: []byte(authSecretValue),
					},
				}
				Expect(k8sClient.Create(ctx, authSecret)).Should(Succeed())

				// Create BitwardenSecret
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
						SecretName:     secretName,
						OrganizationId: orgId.String(),
						SecretMap:      secretMap,
					},
				}
				Expect(k8sClient.Create(ctx, bwSecret)).Should(Succeed())

				// Verify BitwardenSecret was created
				createdSecret := &operatorsv1.BitwardenSecret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, createdSecret)).Should(Succeed())

				// Clean up target secret if it exists
				targetSecret := &corev1.Secret{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, targetSecret)
				if err == nil {
					Expect(k8sClient.Delete(ctx, targetSecret)).Should(Succeed())
				} else if !errors.IsNotFound(err) {
					Fail(fmt.Sprintf("Failed to check target secret: %v", err))
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: namespace,
					},
				}

				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(time.Duration(reconciler.RefreshIntervalSeconds) * time.Second))

				// Verify created secret
				createdTargetSecret := &corev1.Secret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, createdTargetSecret)).Should(Succeed())
				Expect(createdTargetSecret.Labels[controller.LabelBwSecret]).To(Equal(string(bwSecret.UID)))
				Expect(createdTargetSecret.Type).To(Equal(corev1.SecretTypeOpaque))
				Expect(len(createdTargetSecret.Data)).To(Equal(expectedNumOfSecrets))

				// Verify annotations
				Expect(createdTargetSecret.Annotations[controller.AnnotationSyncTime]).NotTo(BeEmpty())
				Expect(createdTargetSecret.Annotations[controller.AnnotationCustomMap]).NotTo(BeEmpty())

				// Verify SuccessfulSync condition and LastSuccessfulSyncTime
				updatedBwSecret := &operatorsv1.BitwardenSecret{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, updatedBwSecret)).Should(Succeed())
				condition := apimeta.FindStatusCondition(updatedBwSecret.Status.Conditions, "SuccessfulSync")
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(updatedBwSecret.Status.LastSuccessfulSyncTime.Time).NotTo(BeZero())
			})
		*/
	})
})
