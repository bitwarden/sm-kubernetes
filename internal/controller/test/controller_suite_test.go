package controller_test

import (
	"testing"

	"github.com/bitwarden/sm-kubernetes/internal/controller/test/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	// Add necessary imports for your project
	//+kubebuilder:scaffold:imports
)

var (
	testContext   *testing.T
	envTestRunner *testutils.EnvTestRunner
)

func TestBitwardenSecretsController(t *testing.T) {
	testContext = t
	RegisterFailHandler(Fail)

	BeforeSuite(func() {
		envTestRunner = testutils.NewEnvTestRunner()
	})

	AfterSuite(func() {
		envTestRunner.Stop()
	})

	RunSpecs(t, "Bitwarden Secrets Controller Suite", Label("controller"))
}
