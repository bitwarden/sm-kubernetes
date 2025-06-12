package controller_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	// Add necessary imports for your project
	//+kubebuilder:scaffold:imports
)

var testContext *testing.T

func TestBitwardenSecretsController(t *testing.T) {
	testContext = t
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bitwarden Secrets Controller Suite")
}
