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

package controller_test

import (
	"time"

	sdk "github.com/bitwarden/sdk-go"
	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/controller"
	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Secret Name Validation Tests", Ordered, func() {
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

	Describe("ValidateSecretKeyName", func() {
		It("should accept valid POSIX-compliant names", func() {
			validNames := []string{
				"DATABASE_PASSWORD",
				"API_KEY",
				"_private_key",
				"secret123",
				"MY_SECRET_2",
				"a",
				"_",
				"test_secret_name_123",
			}

			for _, name := range validNames {
				err := controller.ValidateSecretKeyName(name)
				Expect(err).NotTo(HaveOccurred(), "Expected '%s' to be valid", name)
			}
		})

		It("should reject empty names", func() {
			err := controller.ValidateSecretKeyName("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be empty"))
		})

		It("should reject names starting with a digit", func() {
			err := controller.ValidateSecretKeyName("123secret")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must start with a letter or underscore"))
		})

		It("should reject names with hyphens", func() {
			err := controller.ValidateSecretKeyName("my-secret")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid character"))
			Expect(err.Error()).To(ContainSubstring("-"))
		})

		It("should reject names with spaces", func() {
			err := controller.ValidateSecretKeyName("my secret")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid character"))
		})

		It("should reject names with special characters", func() {
			invalidChars := []string{"@", "#", "$", "%", "^", "&", "*", "(", ")", "+", "=", "[", "]", "{", "}", "|", "\\", ";", ":", "'", "\"", "<", ">", ",", ".", "?", "/"}

			for _, char := range invalidChars {
				name := "secret" + char + "name"
				err := controller.ValidateSecretKeyName(name)
				Expect(err).To(HaveOccurred(), "Expected '%s' to be invalid", name)
				Expect(err.Error()).To(ContainSubstring("invalid character"))
			}
		})
	})

	Describe("UseSecretNames mode with validation", func() {
		It("should successfully sync with valid secret names", func() {
			// Create mock response with valid secret names
			validSecretsData := []sdk.SecretResponse{
				{
					CreationDate:   time.Now().String(),
					ID:             uuid.NewString(),
					Key:            "DATABASE_PASSWORD",
					Value:          "db-secret-value",
					OrganizationID: fixture.OrgId,
				},
				{
					CreationDate:   time.Now().String(),
					ID:             uuid.NewString(),
					Key:            "API_KEY",
					Value:          "api-key-value",
					OrganizationID: fixture.OrgId,
				},
			}
			validSecretsResponse := sdk.SecretsSyncResponse{
				HasChanges: true,
				Secrets:    validSecretsData,
			}

			fixture.SetupDefaultCtrlMocks(false, &validSecretsResponse)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			// Create BitwardenSecret with useSecretNames enabled
			bwSecret, err := fixture.CreateBitwardenSecret(
				testutils.BitwardenSecretName,
				namespace,
				fixture.OrgId,
				testutils.SynchronizedSecretName,
				testutils.AuthSecretName,
				testutils.AuthSecretKey,
				[]operatorsv1.SecretMap{}, // No mapping needed
				false,                     // onlyMappedSecrets = false
			)
			Expect(err).NotTo(HaveOccurred())

			// Enable useSecretNames
			bwSecret.Spec.UseSecretNames = true
			err = fixture.K8sClient.Update(fixture.Ctx, bwSecret)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
			_, err = fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify K8s secret was created with name-based keys
			k8sSecret := &corev1.Secret{}
			err = fixture.K8sClient.Get(fixture.Ctx,
				types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace},
				k8sSecret)
			Expect(err).NotTo(HaveOccurred())

			// Verify keys are secret names, not UUIDs
			Expect(k8sSecret.Data).To(HaveKey("DATABASE_PASSWORD"))
			Expect(k8sSecret.Data).To(HaveKey("API_KEY"))
			Expect(string(k8sSecret.Data["DATABASE_PASSWORD"])).To(Equal("db-secret-value"))
			Expect(string(k8sSecret.Data["API_KEY"])).To(Equal("api-key-value"))
		})

		It("should fail with invalid secret names", func() {
			// Create mock response with invalid secret names
			invalidSecretsData := []sdk.SecretResponse{
				{
					CreationDate:   time.Now().String(),
					ID:             uuid.NewString(),
					Key:            "123-invalid", // Starts with digit
					Value:          "some-value",
					OrganizationID: fixture.OrgId,
				},
				{
					CreationDate:   time.Now().String(),
					ID:             uuid.NewString(),
					Key:            "my-secret", // Contains hyphen
					Value:          "another-value",
					OrganizationID: fixture.OrgId,
				},
			}
			invalidSecretsResponse := sdk.SecretsSyncResponse{
				HasChanges: true,
				Secrets:    invalidSecretsData,
			}

			fixture.SetupDefaultCtrlMocks(false, &invalidSecretsResponse)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			// Create BitwardenSecret with useSecretNames enabled
			bwSecret, err := fixture.CreateBitwardenSecret(
				testutils.BitwardenSecretName,
				namespace,
				fixture.OrgId,
				testutils.SynchronizedSecretName,
				testutils.AuthSecretName,
				testutils.AuthSecretKey,
				[]operatorsv1.SecretMap{},
				false,
			)
			Expect(err).NotTo(HaveOccurred())

			bwSecret.Spec.UseSecretNames = true
			err = fixture.K8sClient.Update(fixture.Ctx, bwSecret)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation - should fail
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
			_, err = fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Invalid secret key names found"))
		})

		It("should fail with duplicate secret names", func() {
			// Create mock response with duplicate secret names
			duplicateId1 := uuid.NewString()
			duplicateId2 := uuid.NewString()
			duplicateSecretsData := []sdk.SecretResponse{
				{
					CreationDate:   time.Now().String(),
					ID:             duplicateId1,
					Key:            "DUPLICATE_NAME",
					Value:          "value1",
					OrganizationID: fixture.OrgId,
				},
				{
					CreationDate:   time.Now().String(),
					ID:             duplicateId2,
					Key:            "DUPLICATE_NAME", // Same name as above
					Value:          "value2",
					OrganizationID: fixture.OrgId,
				},
			}
			duplicateSecretsResponse := sdk.SecretsSyncResponse{
				HasChanges: true,
				Secrets:    duplicateSecretsData,
			}

			fixture.SetupDefaultCtrlMocks(false, &duplicateSecretsResponse)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			// Create BitwardenSecret with useSecretNames enabled
			bwSecret, err := fixture.CreateBitwardenSecret(
				testutils.BitwardenSecretName,
				namespace,
				fixture.OrgId,
				testutils.SynchronizedSecretName,
				testutils.AuthSecretName,
				testutils.AuthSecretKey,
				[]operatorsv1.SecretMap{},
				false,
			)
			Expect(err).NotTo(HaveOccurred())

			bwSecret.Spec.UseSecretNames = true
			err = fixture.K8sClient.Update(fixture.Ctx, bwSecret)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation - should fail
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
			_, err = fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Duplicate secret key names detected"))
			Expect(err.Error()).To(ContainSubstring("DUPLICATE_NAME"))
		})
	})

	Describe("Backward compatibility - UUID mode (default)", func() {
		It("should use UUIDs as keys when useSecretNames is false (default)", func() {
			// Use default fixture setup (useSecretNames = false by default)
			fixture.SetupDefaultCtrlMocks(false, nil)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			// Create BitwardenSecret without setting useSecretNames (defaults to false)
			_, err = fixture.CreateDefaultBitwardenSecret(namespace, fixture.SecretMap)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
			_, err = fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify K8s secret was created with UUID keys (mapped via SecretMap)
			k8sSecret := &corev1.Secret{}
			err = fixture.K8sClient.Get(fixture.Ctx,
				types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace},
				k8sSecret)
			Expect(err).NotTo(HaveOccurred())

			// Verify keys are the mapped names (from SecretMap), not the secret names
			Expect(k8sSecret.Data).To(HaveKey("secret_0_key"))
			Expect(k8sSecret.Data).To(HaveKey("secret_1_key"))
			Expect(k8sSecret.Data).To(HaveKey("secret_2_key"))
		})

		It("should ignore invalid secret names in UUID mode", func() {
			// Create secrets with invalid names that would fail in name mode
			invalidNameSecretsData := []sdk.SecretResponse{
				{
					CreationDate:   time.Now().String(),
					ID:             uuid.NewString(),
					Key:            "123-invalid-name", // Would be invalid in name mode
					Value:          "value1",
					OrganizationID: fixture.OrgId,
				},
				{
					CreationDate:   time.Now().String(),
					ID:             uuid.NewString(),
					Key:            "my-secret", // Would be invalid in name mode
					Value:          "value2",
					OrganizationID: fixture.OrgId,
				},
			}
			invalidNameSecretsResponse := sdk.SecretsSyncResponse{
				HasChanges: true,
				Secrets:    invalidNameSecretsData,
			}

			fixture.SetupDefaultCtrlMocks(false, &invalidNameSecretsResponse)

			_, err := fixture.CreateDefaultAuthSecret(namespace)
			Expect(err).NotTo(HaveOccurred())

			// Create BitwardenSecret with useSecretNames = false (or unset)
			_, err = fixture.CreateBitwardenSecret(
				testutils.BitwardenSecretName,
				namespace,
				fixture.OrgId,
				testutils.SynchronizedSecretName,
				testutils.AuthSecretName,
				testutils.AuthSecretKey,
				[]operatorsv1.SecretMap{},
				false,
			)
			Expect(err).NotTo(HaveOccurred())

			// Trigger reconciliation - should succeed (ignores invalid names in UUID mode)
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: testutils.BitwardenSecretName, Namespace: namespace}}
			_, err = fixture.Reconciler.Reconcile(fixture.Ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify K8s secret was created with UUID keys
			k8sSecret := &corev1.Secret{}
			err = fixture.K8sClient.Get(fixture.Ctx,
				types.NamespacedName{Name: testutils.SynchronizedSecretName, Namespace: namespace},
				k8sSecret)
			Expect(err).NotTo(HaveOccurred())

			// Should have 2 entries with UUID keys
			Expect(len(k8sSecret.Data)).To(Equal(2))
		})
	})
})
