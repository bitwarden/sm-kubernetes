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

package main

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestSettings(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Settings Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
})

var _ = AfterSuite(func() {
	By(" tearing down")
})

var _ = Describe("Get settings", Ordered, func() {

	It("Pulls the default settings", func() {
		os.Setenv("BW_API_URL", "")
		os.Setenv("BW_IDENTITY_API_URL", "")
		os.Setenv("BW_SECRETS_MANAGER_STATE_PATH", "")
		os.Setenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL", "")
		apiUri, identityUri, statePath, refreshInterval, err := GetSettings()
		Expect(*apiUri).Should(Equal("https://api.bitwarden.com"))
		Expect(*identityUri).Should(Equal("https://identity.bitwarden.com"))
		Expect(*statePath).Should(Equal("/var/bitwarden/state"))
		Expect(*refreshInterval).Should(Equal(300))
		Expect(err).Should(BeNil())
	})

	It("Pulls some env settings", func() {
		os.Setenv("BW_API_URL", "https://api.bitwarden.eu")
		os.Setenv("BW_IDENTITY_API_URL", "https://identity.bitwarden.eu")
		os.Setenv("BW_SECRETS_MANAGER_STATE_PATH", "~/state")
		os.Setenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL", "180")
		apiUri, identityUri, statePath, refreshInterval, err := GetSettings()
		Expect(*apiUri).Should(Equal("https://api.bitwarden.eu"))
		Expect(*identityUri).Should(Equal("https://identity.bitwarden.eu"))
		Expect(*statePath).Should(Equal("~/state"))
		Expect(*refreshInterval).Should(Equal(180))
		Expect(err).Should(BeNil())
	})

	It("Fails on bad API URL", func() {
		os.Setenv("BW_API_URL", "https:/api.bitwarden.com")
		os.Setenv("BW_IDENTITY_API_URL", "https://identity.bitwarden.eu")
		os.Setenv("BW_SECRETS_MANAGER_STATE_PATH", "~/state")
		os.Setenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL", "180")

		bwApi, identityApi, statePath, refreshInterval, err := GetSettings()

		Expect(bwApi).Should(BeNil())
		Expect(identityApi).Should(BeNil())
		Expect(statePath).Should(BeNil())
		Expect(refreshInterval).Should(BeNil())
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(Equal("Bitwarden API URL is not valid.  Value supplied: https:/api.bitwarden.com"))

		os.Setenv("BW_API_URL", ".bitwarden.")

		bwApi, identityApi, statePath, refreshInterval, err = GetSettings()

		Expect(bwApi).Should(BeNil())
		Expect(identityApi).Should(BeNil())
		Expect(statePath).Should(BeNil())
		Expect(refreshInterval).Should(BeNil())
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(Equal("parse \".bitwarden.\": invalid URI for request"))
	})

	It("Fails on bad Identity URL", func() {
		os.Setenv("BW_API_URL", "https://identity.bitwarden.eu")
		os.Setenv("BW_IDENTITY_API_URL", "https:/identity.bitwarden.com")
		os.Setenv("BW_SECRETS_MANAGER_STATE_PATH", "~/state")
		os.Setenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL", "180")

		bwApi, identityApi, statePath, refreshInterval, err := GetSettings()

		Expect(bwApi).Should(BeNil())
		Expect(identityApi).Should(BeNil())
		Expect(statePath).Should(BeNil())
		Expect(refreshInterval).Should(BeNil())
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(Equal("Bitwarden Identity URL is not valid.  Value supplied: https:/identity.bitwarden.com"))

		os.Setenv("BW_IDENTITY_API_URL", ".bitwarden.")

		bwApi, identityApi, statePath, refreshInterval, err = GetSettings()

		Expect(bwApi).Should(BeNil())
		Expect(identityApi).Should(BeNil())
		Expect(statePath).Should(BeNil())
		Expect(refreshInterval).Should(BeNil())
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(Equal("parse \".bitwarden.\": invalid URI for request"))
	})

	It("Pulls with defaulted refresh interval", func() {
		os.Setenv("BW_API_URL", "")
		os.Setenv("BW_IDENTITY_API_URL", "")
		os.Setenv("BW_SECRETS_MANAGER_STATE_PATH", "")
		os.Setenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL", "179")
		apiUri, identityUri, statePath, refreshInterval, err := GetSettings()
		Expect(*apiUri).Should(Equal("https://api.bitwarden.com"))
		Expect(*identityUri).Should(Equal("https://identity.bitwarden.com"))
		Expect(*statePath).Should(Equal("/var/bitwarden/state"))
		Expect(*refreshInterval).Should(Equal(300))
		Expect(err).Should(BeNil())

		os.Setenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL", "abc")
		apiUri, identityUri, statePath, refreshInterval, err = GetSettings()
		Expect(*apiUri).Should(Equal("https://api.bitwarden.com"))
		Expect(*identityUri).Should(Equal("https://identity.bitwarden.com"))
		Expect(*statePath).Should(Equal("/var/bitwarden/state"))
		Expect(*refreshInterval).Should(Equal(300))
		Expect(err).Should(BeNil())
	})
})
