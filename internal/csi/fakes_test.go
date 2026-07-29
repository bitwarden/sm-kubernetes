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

package csi

import (
	"sync"
	"time"

	sdk "github.com/bitwarden/sdk-go/v2"

	"github.com/bitwarden/sm-kubernetes/internal/sm"
)

// syncFunc is the test-supplied behavior of a fake SecretsInterface.Sync
// call.
type syncFunc func(organizationID string, lastSyncedDate *time.Time) (*sdk.SecretsSyncResponse, error)

// fakeSecrets is a minimal sdk.SecretsInterface stand-in that only
// implements Sync (the only method this package's Mount logic calls),
// counting invocations for test assertions. The remaining methods are
// unused by production code and simply return zero values.
type fakeSecrets struct {
	mu    sync.Mutex
	calls int
	fn    syncFunc
}

func (f *fakeSecrets) Sync(organizationID string, lastSyncedDate *time.Time) (*sdk.SecretsSyncResponse, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()

	return f.fn(organizationID, lastSyncedDate)
}

func (f *fakeSecrets) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

func (f *fakeSecrets) Create(string, string, string, string, []string) (*sdk.SecretResponse, error) {
	return nil, nil
}

func (f *fakeSecrets) List(string) (*sdk.SecretIdentifiersResponse, error) { return nil, nil }

func (f *fakeSecrets) Get(string) (*sdk.SecretResponse, error) { return nil, nil }

func (f *fakeSecrets) GetByIDS([]string) (*sdk.SecretsResponse, error) { return nil, nil }

func (f *fakeSecrets) Update(string, string, string, string, string, []string) (*sdk.SecretResponse, error) {
	return nil, nil
}

func (f *fakeSecrets) Delete([]string) (*sdk.SecretsDeleteResponse, error) { return nil, nil }

// fakeProjects and fakeGenerators are unused-but-required implementations of
// the remaining sdk.BitwardenClientInterface surface.
type fakeProjects struct{}

func (fakeProjects) Create(string, string) (*sdk.ProjectResponse, error) { return nil, nil }
func (fakeProjects) List(string) (*sdk.ProjectsResponse, error)          { return nil, nil }
func (fakeProjects) Get(string) (*sdk.ProjectResponse, error)            { return nil, nil }
func (fakeProjects) Update(string, string, string) (*sdk.ProjectResponse, error) {
	return nil, nil
}
func (fakeProjects) Delete([]string) (*sdk.ProjectsDeleteResponse, error) { return nil, nil }

type fakeGenerators struct{}

func (fakeGenerators) GeneratePassword(sdk.PasswordGeneratorRequest) (*string, error) {
	return nil, nil
}

// fakeBitwardenClient is a minimal sdk.BitwardenClientInterface stand-in.
// AccessTokenLogin always succeeds unless loginErr is set; Close just
// records that it was called.
type fakeBitwardenClient struct {
	secrets *fakeSecrets

	mu        sync.Mutex
	loginErr  error
	loginCall int
	closed    bool
}

func newFakeBitwardenClient(fn syncFunc) *fakeBitwardenClient {
	return &fakeBitwardenClient{secrets: &fakeSecrets{fn: fn}}
}

func (c *fakeBitwardenClient) AccessTokenLogin(_ string, _ *string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.loginCall++

	return c.loginErr
}

func (c *fakeBitwardenClient) Projects() sdk.ProjectsInterface { return fakeProjects{} }

func (c *fakeBitwardenClient) Secrets() sdk.SecretsInterface { return c.secrets }

func (c *fakeBitwardenClient) Generators() sdk.GeneratorsInterface { return fakeGenerators{} }

func (c *fakeBitwardenClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
}

func (c *fakeBitwardenClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closed
}

// fakeBitwardenClientFactory is a minimal sm.BitwardenClientFactory
// implementation. Like the real sm.BitwardenClientFactoryImp, constructing
// it is cheap; the actual (fake) SDK client is only created - and recorded
// on the owning fakeClientFactorySet - when GetBitwardenClient is called,
// mirroring how the real factory only creates the underlying SDK client on
// demand.
type fakeBitwardenClientFactory struct {
	set *fakeClientFactorySet
}

func (f *fakeBitwardenClientFactory) GetBitwardenClient() (sdk.BitwardenClientInterface, error) {
	return f.set.newClient(), nil
}

func (f *fakeBitwardenClientFactory) GetApiUrl() string { return "https://api.example.test" }
func (f *fakeBitwardenClientFactory) GetIdentityApiUrl() string {
	return "https://identity.example.test"
}

// fakeClientFactorySet is a test helper that builds a clientFactoryFunc
// which fabricates a lightweight fakeBitwardenClientFactory every time it's
// invoked (matching the real factory's cheap construction), while tracking
// every fake SDK client that has actually been created (i.e. every time
// GetBitwardenClient was called) so tests can assert on cache behavior
// (e.g. that a session was reused rather than recreated, or that an evicted
// session was Close()'d).
type fakeClientFactorySet struct {
	mu       sync.Mutex
	fn       syncFunc
	loginErr error
	clients  []*fakeBitwardenClient
}

func newFakeClientFactory(fn syncFunc) *fakeClientFactorySet {
	return &fakeClientFactorySet{fn: fn}
}

func (s *fakeClientFactorySet) newFactory(_ string, _ string) sm.BitwardenClientFactory {
	return &fakeBitwardenClientFactory{set: s}
}

// setLoginErr configures every fake client subsequently created by this
// factory set to fail AccessTokenLogin with err.
func (s *fakeClientFactorySet) setLoginErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loginErr = err
}

func (s *fakeClientFactorySet) newClient() *fakeBitwardenClient {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := newFakeBitwardenClient(s.fn)
	client.loginErr = s.loginErr
	s.clients = append(s.clients, client)

	return client
}

// creationCalls returns how many distinct fake clients (i.e. distinct
// logged-in sessions) have been created.
func (s *fakeClientFactorySet) creationCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.clients)
}

// syncCalls returns the total number of Secrets().Sync calls across every
// client this factory has created.
func (s *fakeClientFactorySet) syncCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0
	for _, c := range s.clients {
		total += c.secrets.callCount()
	}

	return total
}

// clientAt returns the nth (0-indexed) client this factory has created.
func (s *fakeClientFactorySet) clientAt(i int) *fakeBitwardenClient {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.clients[i]
}
