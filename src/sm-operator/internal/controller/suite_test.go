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
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc
var namespace string

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	namespace = "bitwarden-ns"

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = operatorsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&BitwardenSecretReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	//+kubebuilder:scaffold:scheme
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	ctx, cancel = context.WithCancel(context.TODO())

	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Bitwarden Secrets Controller", func() {
	Context("When a Bitwarden Secret object is created without a mapping", func() {
		It("Creates a synchronized K8s secret without a mapping", func() {
			authSecretName := "bw-secret-sample-1"
			authSecretKey := "token-key"
			authSecretValue := "abc-123"

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
		})
	})
})

// func TestBitwardenSecretsControllerHappy(t *testing.T) {

// 	// orgId := uuid.New()
// 	// name := "bw-secret"
// 	// secretName := "bitwarden-k8s-secret-sample"

// 	authSecretName := "bw-secret-sample-1"
// 	authSecretKey := "token-key"
// 	authSecretValue := "abc-123"

// 	authSecret := corev1.Secret{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:        authSecretName,
// 			Namespace:   namespace,
// 			Labels:      map[string]string{},
// 			Annotations: map[string]string{},
// 		},
// 		TypeMeta: metav1.TypeMeta{
// 			Kind:       "Secret",
// 			APIVersion: "v1",
// 		},
// 		Type: corev1.SecretTypeOpaque,
// 		Data: map[string][]byte{authSecretKey: []byte(authSecretValue)},
// 	}

// 	Expect(k8sClient.Create(ctx, &authSecret)).Should(Succeed())

// bwSecret := operatorsv1.BitwardenSecret {
//     ObjectMeta: metav1.ObjectMeta{
//         Name:     name,
//         Namespace: namespace,
//         Labels: map[string]string{
//             "label-key": "label-value",
//         },
//     },
// 	Spec: operatorsv1.BitwardenSecretSpec {
// 		OrganizationId: orgId.String(),
// 		SecretName: secretName,
// 		AuthToken: operatorsv1.AuthToken{
// 			SecretName: authSecretName,
// 			SecretKey: authSecretKey,
// 		},
// 	},
// }

// // Create a fake client to mock API calls.
// cl := fake.NewFakeClient(objs...)

// // List Memcached objects filtering by labels
// opt := client.MatchingLabels(map[string]string{"label-key": "label-value"})
// memcachedList := &cachev1alpha1.MemcachedList{}
// err := cl.List(context.TODO(), memcachedList, opt)
// if err != nil {
//     t.Fatalf("list memcached: (%v)", err)
// }

// }
