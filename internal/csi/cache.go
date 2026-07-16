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
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	sdk "github.com/bitwarden/sdk-go/v2"
	"github.com/bitwarden/sm-kubernetes/internal/sm"
)

const (
	// defaultCacheMaxEntries bounds the number of distinct (organizationId,
	// token) sessions the provider keeps logged in at once. This runs as a
	// DaemonSet handling Mount calls for many pods/volumes, so the cache is
	// bounded to avoid unbounded growth of logged-in SDK clients and their
	// on-disk state directories.
	defaultCacheMaxEntries = 256

	// defaultCacheIdleTimeout is how long a cache entry may go unused before
	// it is eligible for idle eviction, freeing its underlying SDK client and
	// state directory.
	defaultCacheIdleTimeout = 30 * time.Minute

	// syncMaxAttempts and syncBaseBackoff configure the exponential backoff
	// applied to Secrets Manager Sync calls (and the initial login) on
	// failure.
	syncMaxAttempts = 4
	syncBaseBackoff = 250 * time.Millisecond
)

// clientFactoryFunc constructs an sm.BitwardenClientFactory bound to the
// given API/Identity URLs. It exists so tests can substitute a fake factory
// without contacting the real Bitwarden SDK.
type clientFactoryFunc func(apiURL, identityURL string) sm.BitwardenClientFactory

// cacheKey identifies a cached Secrets Manager session. Per the design goal
// of never persisting or logging raw token values, the token itself is
// reduced to a SHA-256 digest before being used as (part of) a map key.
type cacheKey struct {
	orgID     string
	tokenHash string
}

// smClientCache is an in-process, bounded cache of logged-in Secrets Manager
// sessions keyed by (organizationId, sha256(token)). It exists because this
// provider runs as a DaemonSet handling many concurrent Mount calls -
// including one roughly every rotation interval per mounted volume - so
// logging in and re-syncing from scratch on every call would be wasteful and
// slow. Entries are evicted on a bounded LRU basis and after an idle
// timeout; eviction closes the underlying SDK client and removes its
// isolated state directory.
type smClientCache struct {
	mu      sync.Mutex
	entries map[cacheKey]*list.Element // value is *smCacheEntry
	lru     *list.List

	maxEntries  int
	idleTimeout time.Duration

	// baseStateDir is the parent directory under which each cache entry gets
	// its own isolated subdirectory for the SDK's on-disk login state file.
	// Isolating state directories per entry (rather than sharing one
	// statePath, as the controller does) avoids concurrent-login file races
	// between sessions for different organizations/tokens.
	baseStateDir string

	newFactory clientFactoryFunc
}

// newSMClientCache constructs an smClientCache. baseStateDir is created (if
// necessary) lazily as entries are added.
func newSMClientCache(baseStateDir string, maxEntries int, idleTimeout time.Duration, newFactory clientFactoryFunc) *smClientCache {
	return &smClientCache{
		entries:      make(map[cacheKey]*list.Element),
		lru:          list.New(),
		maxEntries:   maxEntries,
		idleTimeout:  idleTimeout,
		baseStateDir: baseStateDir,
		newFactory:   newFactory,
	}
}

// hashToken reduces token to a hex-encoded SHA-256 digest so it is never
// retained or logged in plaintext as a cache key.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// getOrCreate returns the cache entry for (orgID, token), creating one (with
// its own isolated state directory and SDK client factory) if none exists
// yet. The returned entry is not necessarily logged in or synced yet; that
// happens lazily and cooperatively in (*smCacheEntry).sync.
func (c *smClientCache) getOrCreate(orgID, token, apiURL, identityURL string) (*smCacheEntry, error) {
	key := cacheKey{orgID: orgID, tokenHash: hashToken(token)}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictIdleLocked()

	if elem, ok := c.entries[key]; ok {
		c.lru.MoveToFront(elem)
		entry := elem.Value.(*smCacheEntry)
		entry.touch()
		return entry, nil
	}

	for c.lru.Len() >= c.maxEntries {
		c.evictOldestLocked()
	}

	stateDir := filepath.Join(c.baseStateDir, uuid.NewString())
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create isolated state directory: %w", err)
	}

	factory := c.newFactory(apiURL, identityURL)
	entry := newSMCacheEntry(key, stateDir, factory, token)
	entry.touch()

	elem := c.lru.PushFront(entry)
	c.entries[key] = elem

	return entry, nil
}

// evictIdleLocked removes and cleans up entries that have not been used
// within idleTimeout. c.mu must be held.
func (c *smClientCache) evictIdleLocked() {
	if c.idleTimeout <= 0 {
		return
	}

	now := time.Now()

	for elem := c.lru.Back(); elem != nil; {
		entry := elem.Value.(*smCacheEntry)
		if now.Sub(entry.lastUsed()) <= c.idleTimeout {
			break
		}

		prev := elem.Prev()
		c.removeLocked(elem)
		elem = prev
	}
}

// evictOldestLocked evicts the least-recently-used entry. c.mu must be held.
func (c *smClientCache) evictOldestLocked() {
	if elem := c.lru.Back(); elem != nil {
		c.removeLocked(elem)
	}
}

// removeLocked removes elem from the cache and releases its resources.
// c.mu must be held.
func (c *smClientCache) removeLocked(elem *list.Element) {
	entry := elem.Value.(*smCacheEntry)
	delete(c.entries, entry.key)
	c.lru.Remove(elem)
	entry.closeAndCleanup()
}

// Close evicts and cleans up every cache entry. It is intended to be called
// once, on provider shutdown.
func (c *smClientCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for elem := c.lru.Front(); elem != nil; {
		next := elem.Next()
		c.removeLocked(elem)
		elem = next
	}
}

// smCacheEntry is a single cached Secrets Manager session: a logged-in SDK
// client (created lazily) plus the most recently synced secrets, keyed by
// both Secrets Manager secret ID and by secret name/key.
type smCacheEntry struct {
	key      cacheKey
	stateDir string
	factory  sm.BitwardenClientFactory
	token    string

	// lastUsedAt is only ever read/written with lastUsedMu held; it is
	// updated by the cache on every getOrCreate hit, independent of the
	// syncing/login state below.
	lastUsedMu sync.Mutex
	lastUsedAt time.Time

	// initMu guards client/loggedIn, which are set exactly once (the first
	// successful login for this entry).
	initMu   sync.Mutex
	client   sdk.BitwardenClientInterface
	loggedIn bool

	// mu guards the synced secrets snapshot and single-flight state below.
	mu       sync.Mutex
	byID     map[string]sdk.SecretResponse
	byName   map[string][]sdk.SecretResponse
	lastSync *time.Time
	inflight *syncCall
}

// syncCall represents a single in-flight (*smCacheEntry).sync call. Other
// concurrent callers wait on done and reuse its result instead of issuing
// their own Secrets Manager Sync request; this is what collapses many
// concurrent Mount calls for the same (organizationId, token) into a single
// Sync call.
type syncCall struct {
	done chan struct{}
	err  error
}

func newSMCacheEntry(key cacheKey, stateDir string, factory sm.BitwardenClientFactory, token string) *smCacheEntry {
	return &smCacheEntry{
		key:      key,
		stateDir: stateDir,
		factory:  factory,
		token:    token,
	}
}

func (e *smCacheEntry) touch() {
	e.lastUsedMu.Lock()
	e.lastUsedAt = time.Now()
	e.lastUsedMu.Unlock()
}

func (e *smCacheEntry) lastUsed() time.Time {
	e.lastUsedMu.Lock()
	defer e.lastUsedMu.Unlock()
	return e.lastUsedAt
}

// closeAndCleanup releases the entry's SDK client (if one was created) and
// removes its isolated state directory. It is called by the cache when the
// entry is evicted.
func (e *smCacheEntry) closeAndCleanup() {
	e.initMu.Lock()
	if e.client != nil {
		e.client.Close()
	}
	e.initMu.Unlock()

	_ = os.RemoveAll(e.stateDir)
}

// sync fetches the latest secrets for this entry's organization, logging in
// first if necessary, and returns a snapshot indexed by secret ID and by
// secret name/key (the latter may map to more than one secret, since
// Secrets Manager secret names are not guaranteed unique).
//
// Concurrent callers collapse into a single Secrets Manager Sync call: if a
// sync is already in flight for this entry, callers wait for it and reuse
// its result rather than starting a redundant one.
func (e *smCacheEntry) sync(ctx context.Context) (map[string]sdk.SecretResponse, map[string][]sdk.SecretResponse, error) {
	e.mu.Lock()
	if call := e.inflight; call != nil {
		e.mu.Unlock()

		select {
		case <-call.done:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}

		if call.err != nil {
			return nil, nil, call.err
		}

		e.mu.Lock()
		byID, byName := e.byID, e.byName
		e.mu.Unlock()

		return byID, byName, nil
	}

	call := &syncCall{done: make(chan struct{})}
	e.inflight = call
	e.mu.Unlock()

	err := e.doSync(ctx)

	e.mu.Lock()
	e.inflight = nil
	byID, byName := e.byID, e.byName
	e.mu.Unlock()

	call.err = err
	close(call.done)

	if err != nil {
		return nil, nil, err
	}

	return byID, byName, nil
}

// doSync performs the actual login-if-needed + Secrets Manager Sync work
// for sync. It must only ever be run by one goroutine at a time per entry,
// which sync's single-flight gating above guarantees.
func (e *smCacheEntry) doSync(ctx context.Context) error {
	if err := e.ensureLoggedIn(ctx); err != nil {
		return err
	}

	e.mu.Lock()
	lastSync := e.lastSync
	haveSynced := e.byID != nil
	e.mu.Unlock()

	var resp *sdk.SecretsSyncResponse

	err := withBackoff(ctx, syncMaxAttempts, syncBaseBackoff, func() error {
		var syncErr error
		resp, syncErr = e.client.Secrets().Sync(e.key.orgID, lastSync)
		return syncErr
	})
	if err != nil {
		return fmt.Errorf("secrets manager sync failed: %w", err)
	}

	now := time.Now().UTC()

	e.mu.Lock()
	defer e.mu.Unlock()

	// A response with no changes since lastSync omits Secrets entirely; keep
	// serving the previous snapshot in that case so mounted files stay
	// stable. Always populate on the very first sync for this entry,
	// regardless of HasChanges, so a first Mount never returns an empty
	// snapshot.
	if resp != nil && (resp.HasChanges || !haveSynced) {
		byID := make(map[string]sdk.SecretResponse, len(resp.Secrets))
		byName := make(map[string][]sdk.SecretResponse, len(resp.Secrets))

		for _, secret := range resp.Secrets {
			byID[secret.ID] = secret
			byName[secret.Key] = append(byName[secret.Key], secret)
		}

		e.byID = byID
		e.byName = byName
	}

	e.lastSync = &now

	return nil
}

// ensureLoggedIn performs the (at most once) AccessTokenLogin for this
// entry's client, creating the client on first use.
func (e *smCacheEntry) ensureLoggedIn(ctx context.Context) error {
	e.initMu.Lock()
	defer e.initMu.Unlock()

	if e.loggedIn {
		return nil
	}

	client, err := e.factory.GetBitwardenClient()
	if err != nil {
		return fmt.Errorf("failed to create bitwarden client: %w", err)
	}

	statePath := filepath.Join(e.stateDir, "state.json")

	if err := withBackoff(ctx, syncMaxAttempts, syncBaseBackoff, func() error {
		return client.AccessTokenLogin(e.token, &statePath)
	}); err != nil {
		client.Close()
		return fmt.Errorf("failed to authenticate to bitwarden secrets manager: %w", err)
	}

	e.client = client
	e.loggedIn = true

	return nil
}

// withBackoff calls fn until it succeeds, ctx is done, or maxAttempts have
// been made, applying a simple exponential backoff (doubling from
// baseDelay) between attempts. It returns the error from the final attempt.
func withBackoff(ctx context.Context, maxAttempts int, baseDelay time.Duration, fn func() error) error {
	var err error

	delay := baseDelay

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}

		if attempt == maxAttempts {
			break
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return err
		}

		delay *= 2
	}

	return err
}
