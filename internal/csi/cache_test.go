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
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	sdk "github.com/bitwarden/sdk-go/v2"
)

var errBoom = errors.New("boom")

func noopSync(string, *time.Time) (*sdk.SecretsSyncResponse, error) {
	return &sdk.SecretsSyncResponse{HasChanges: true, Secrets: nil}, nil
}

func newTestCache(t *testing.T, maxEntries int, idleTimeout time.Duration, factory *fakeClientFactorySet) *smClientCache {
	t.Helper()

	baseDir, err := os.MkdirTemp("", "csi-cache-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })

	return newSMClientCache(baseDir, maxEntries, idleTimeout, factory.newFactory)
}

// TestCacheGetOrCreateReusesEntryForSameKey verifies that repeated calls for
// the same (organizationId, token) reuse a single cached session instead of
// creating a new logged-in client each time.
func TestCacheGetOrCreateReusesEntryForSameKey(t *testing.T) {
	factory := newFakeClientFactory(noopSync)
	cache := newTestCache(t, defaultCacheMaxEntries, defaultCacheIdleTimeout, factory)

	entry1, err := cache.getOrCreate("org-1", "token-1", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	// Trigger client creation/login so reuse (or lack thereof) is
	// observable via creationCalls.
	if _, _, err := entry1.sync(context.Background()); err != nil {
		t.Fatalf("sync returned unexpected error: %v", err)
	}

	entry2, err := cache.getOrCreate("org-1", "token-1", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	if entry1 != entry2 {
		t.Error("getOrCreate returned a different entry for the same (org, token); expected the cached one to be reused")
	}

	if _, _, err := entry2.sync(context.Background()); err != nil {
		t.Fatalf("second sync returned unexpected error: %v", err)
	}

	if factory.creationCalls() != 1 {
		t.Errorf("client factory invoked %d times, want 1 (the logged-in session should be reused, not recreated)", factory.creationCalls())
	}
}

// TestCacheGetOrCreateSeparatesDifferentKeys verifies that different tokens
// (even for the same organization) get distinct sessions with isolated
// state directories, so concurrent logins for different callers can never
// race on the same on-disk state file.
func TestCacheGetOrCreateSeparatesDifferentKeys(t *testing.T) {
	factory := newFakeClientFactory(noopSync)
	cache := newTestCache(t, defaultCacheMaxEntries, defaultCacheIdleTimeout, factory)

	entryA, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	entryB, err := cache.getOrCreate("org-1", "token-b", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	if entryA == entryB {
		t.Fatal("getOrCreate returned the same entry for different tokens")
	}

	if entryA.stateDir == entryB.stateDir {
		t.Errorf("entries for different tokens share a state directory: %q", entryA.stateDir)
	}

	if factory.creationCalls() != 0 {
		// Client creation is lazy: it only happens on first sync, not on
		// getOrCreate.
		t.Errorf("client factory invoked %d times before any sync, want 0", factory.creationCalls())
	}
}

// TestCacheBoundedEvictionClosesOldestEntry verifies that once the cache is
// at capacity, adding a new key evicts the least-recently-used entry and
// releases its resources (closes its client, removes its state directory).
func TestCacheBoundedEvictionClosesOldestEntry(t *testing.T) {
	factory := newFakeClientFactory(noopSync)
	cache := newTestCache(t, 1, defaultCacheIdleTimeout, factory)

	oldEntry, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	// Force client creation (and thus something for eviction to close) via
	// a sync.
	if _, _, err := oldEntry.sync(context.Background()); err != nil {
		t.Fatalf("sync returned unexpected error: %v", err)
	}

	oldStateDir := oldEntry.stateDir
	oldClient := factory.clientAt(0)

	if _, err := os.Stat(oldStateDir); err != nil {
		t.Fatalf("expected state dir to exist before eviction: %v", err)
	}

	// A second, different key exceeds the cache's capacity of 1, evicting
	// the first entry.
	if _, err := cache.getOrCreate("org-2", "token-b", "https://api", "https://identity"); err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	if !oldClient.isClosed() {
		t.Error("evicted entry's client was not closed")
	}

	if _, err := os.Stat(oldStateDir); !os.IsNotExist(err) {
		t.Errorf("expected evicted entry's state dir to be removed, stat err = %v", err)
	}

	// The evicted key should now be a cache miss requiring a fresh client.
	recreatedEntry, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	if _, _, err := recreatedEntry.sync(context.Background()); err != nil {
		t.Fatalf("sync on recreated entry returned unexpected error: %v", err)
	}

	if factory.creationCalls() != 2 {
		t.Errorf("client factory invoked %d times, want 2 (one per session, since the first was evicted)", factory.creationCalls())
	}
}

// TestCacheIdleEviction verifies that an entry unused for longer than the
// configured idle timeout is evicted (and its resources released) even
// without the cache being at capacity.
func TestCacheIdleEviction(t *testing.T) {
	factory := newFakeClientFactory(noopSync)
	cache := newTestCache(t, defaultCacheMaxEntries, time.Millisecond, factory)

	entry, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	if _, _, err := entry.sync(context.Background()); err != nil {
		t.Fatalf("sync returned unexpected error: %v", err)
	}

	oldClient := factory.clientAt(0)

	time.Sleep(10 * time.Millisecond)

	// Any cache access sweeps idle entries first.
	newEntry, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	if newEntry == entry {
		t.Error("expected a fresh entry after idle eviction, got the same one back")
	}

	if !oldClient.isClosed() {
		t.Error("idle-evicted entry's client was not closed")
	}
}

// TestEntrySyncSingleFlight verifies that concurrent sync() calls for the
// same cache entry collapse into a single underlying Secrets Manager Sync
// call, so a burst of concurrent Mount calls for the same (organizationId,
// token) doesn't fan out into N Sync calls.
func TestEntrySyncSingleFlight(t *testing.T) {
	const concurrency = 20

	release := make(chan struct{})
	entered := make(chan struct{}, concurrency)

	factory := newFakeClientFactory(func(string, *time.Time) (*sdk.SecretsSyncResponse, error) {
		entered <- struct{}{}
		<-release
		return &sdk.SecretsSyncResponse{HasChanges: true, Secrets: []sdk.SecretResponse{{ID: "id-1", Value: "v1"}}}, nil
	})

	cache := newTestCache(t, defaultCacheMaxEntries, defaultCacheIdleTimeout, factory)

	entry, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	var wg sync.WaitGroup

	results := make([]map[string]sdk.SecretResponse, concurrency)
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			byID, _, err := entry.sync(context.Background())
			results[i] = byID
			errs[i] = err
		}(i)
	}

	// Wait for exactly one Sync call to actually start.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the underlying Sync call to start")
	}

	// Give any (incorrect) additional concurrent Sync calls a chance to
	// start before we unblock the first one; there should not be any.
	select {
	case <-entered:
		t.Fatal("more than one underlying Sync call started concurrently")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("sync[%d] returned unexpected error: %v", i, err)
		}

		if results[i]["id-1"].Value != "v1" {
			t.Errorf("sync[%d] result = %+v, want secret id-1 with value v1", i, results[i])
		}
	}

	if factory.syncCalls() != 1 {
		t.Errorf("underlying Secrets().Sync called %d times, want exactly 1 for %d concurrent callers", factory.syncCalls(), concurrency)
	}
}

// TestEntrySyncKeepsPreviousSnapshotWhenNoChanges verifies that when
// Secrets Manager reports no changes on a subsequent sync, the entry keeps
// serving its previous snapshot instead of clearing it.
func TestEntrySyncKeepsPreviousSnapshotWhenNoChanges(t *testing.T) {
	calls := 0

	factory := newFakeClientFactory(func(string, *time.Time) (*sdk.SecretsSyncResponse, error) {
		calls++
		if calls == 1 {
			return &sdk.SecretsSyncResponse{HasChanges: true, Secrets: []sdk.SecretResponse{{ID: "id-1", Value: "v1"}}}, nil
		}

		return &sdk.SecretsSyncResponse{HasChanges: false, Secrets: nil}, nil
	})

	cache := newTestCache(t, defaultCacheMaxEntries, defaultCacheIdleTimeout, factory)

	entry, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	byID1, _, err := entry.sync(context.Background())
	if err != nil {
		t.Fatalf("first sync returned unexpected error: %v", err)
	}

	if byID1["id-1"].Value != "v1" {
		t.Fatalf("first sync result = %+v, want secret id-1 with value v1", byID1)
	}

	byID2, _, err := entry.sync(context.Background())
	if err != nil {
		t.Fatalf("second sync returned unexpected error: %v", err)
	}

	if byID2["id-1"].Value != "v1" {
		t.Errorf("second sync (no changes) result = %+v, want previous snapshot with secret id-1 = v1", byID2)
	}
}

// TestEntrySyncPropagatesLoginFailure verifies that a login failure surfaces
// as an error from sync rather than silently proceeding. It uses a short
// context deadline so the login's exponential backoff retries abort
// quickly instead of running for several seconds.
func TestEntrySyncPropagatesLoginFailure(t *testing.T) {
	factory := newFakeClientFactory(noopSync)
	factory.setLoginErr(errBoom)

	cache := newTestCache(t, defaultCacheMaxEntries, defaultCacheIdleTimeout, factory)

	entry, err := cache.getOrCreate("org-1", "token-a", "https://api", "https://identity")
	if err != nil {
		t.Fatalf("getOrCreate returned unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, _, err := entry.sync(ctx); err == nil {
		t.Fatal("sync unexpectedly succeeded despite a login failure")
	}
}
