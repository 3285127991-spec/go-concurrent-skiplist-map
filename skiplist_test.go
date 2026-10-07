package skiplistmap

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func mustPut(t *testing.T, m *Map, key, value string) {
	t.Helper()
	if err := m.Put(key, value); err != nil {
		t.Fatalf("Put(%q, %q): %v", key, value, err)
	}
}

// assertLevelInvariants walks every index level and verifies the structural
// invariants of the skip list: each level is strictly sorted, higher levels
// are subsets of lower ones, nothing lives above the current height, and the
// level-0 node count matches Len.
func assertLevelInvariants(t *testing.T, m *Map) {
	t.Helper()
	var below map[string]bool
	zeroCount := 0
	for i := 0; i < maxLevel; i++ {
		var keys []string
		for x := m.head.next[i]; x != nil; x = x.next[i] {
			keys = append(keys, x.key)
		}
		if i >= m.level && len(keys) != 0 {
			t.Fatalf("level %d is above current height %d but has %d nodes", i, m.level, len(keys))
		}
		for j := 1; j < len(keys); j++ {
			if keys[j-1] >= keys[j] {
				t.Fatalf("level %d not strictly sorted: %q >= %q", i, keys[j-1], keys[j])
			}
		}
		if below != nil {
			for _, k := range keys {
				if !below[k] {
					t.Fatalf("level %d key %q missing from level below (dangling index)", i, k)
				}
			}
		}
		if i == 0 {
			zeroCount = len(keys)
		}
		below = make(map[string]bool, len(keys))
		for _, k := range keys {
			below[k] = true
		}
	}
	if zeroCount != m.Len() {
		t.Fatalf("level 0 has %d nodes but Len() = %d", zeroCount, m.Len())
	}
}

func assertSnapshot(t *testing.T, m *Map, want []Entry) {
	t.Helper()
	got := m.Snapshot()
	if len(got) != len(want) {
		t.Fatalf("Snapshot() has %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, e := range want {
		if got[i] != e {
			t.Fatalf("Snapshot()[%d] = %+v, want %+v", i, got[i], e)
		}
	}
}

func TestPutGetAndOrder(t *testing.T) {
	m := New()
	kvs := []Entry{
		{"delta", "4"},
		{"alpha", "1"},
		{"echo", "5"},
		{"bravo", "2"},
		{"charlie", "3"},
	}
	for _, kv := range kvs {
		mustPut(t, m, kv.Key, kv.Value)
	}
	if got := m.Len(); got != len(kvs) {
		t.Fatalf("Len() = %d, want %d", got, len(kvs))
	}
	for _, kv := range kvs {
		if v, ok := m.Get(kv.Key); !ok || v != kv.Value {
			t.Fatalf("Get(%q) = %q, %v; want %q, true", kv.Key, v, ok, kv.Value)
		}
	}
	if _, ok := m.Get("nope"); ok {
		t.Fatal("Get(nope) reported found")
	}
	assertSnapshot(t, m, []Entry{
		{"alpha", "1"},
		{"bravo", "2"},
		{"charlie", "3"},
		{"delta", "4"},
		{"echo", "5"},
	})
	assertLevelInvariants(t, m)
}

func TestPutUpdateExisting(t *testing.T) {
	m := New()
	mustPut(t, m, "key", "v1")
	mustPut(t, m, "key", "v2")
	if got := m.Len(); got != 1 {
		t.Fatalf("Len() = %d after re-put, want 1 (no new node)", got)
	}
	if v, ok := m.Get("key"); !ok || v != "v2" {
		t.Fatalf("Get(key) = %q, %v; want v2, true", v, ok)
	}
	assertSnapshot(t, m, []Entry{{"key", "v2"}})
	assertLevelInvariants(t, m)
}

func TestLowerBound(t *testing.T) {
	m := New()
	if _, ok := m.LowerBound("a"); ok {
		t.Fatal("LowerBound on empty map reported found")
	}
	mustPut(t, m, "apple", "1")
	mustPut(t, m, "cherry", "2")
	mustPut(t, m, "grape", "3")

	cases := []struct {
		target  string
		wantKey string
		wantOk  bool
	}{
		{"", "apple", true},      // before everything
		{"apple", "apple", true}, // exact hit
		{"app", "apple", true},   // prefix of first key
		{"banana", "cherry", true},
		{"cherry", "cherry", true},
		{"dates", "grape", true},
		{"grape", "grape", true},
		{"zzz", "", false}, // past the end
	}
	for _, c := range cases {
		e, ok := m.LowerBound(c.target)
		if ok != c.wantOk || (ok && e.Key != c.wantKey) {
			t.Errorf("LowerBound(%q) = %+v, %v; want key %q, %v", c.target, e, ok, c.wantKey, c.wantOk)
		}
	}
}

func TestRange(t *testing.T) {
	m := New()
	keys := []string{"apple", "banana", "cherry", "date", "fig", "grape"}
	for i, k := range keys {
		mustPut(t, m, k, fmt.Sprintf("v%d", i))
	}

	got, err := m.Range("banana", "grape")
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	wantKeys := []string{"banana", "cherry", "date", "fig"} // [start, end)
	if len(got) != len(wantKeys) {
		t.Fatalf("Range returned %d entries, want %d: %v", len(got), len(wantKeys), got)
	}
	for i, k := range wantKeys {
		if got[i].Key != k {
			t.Fatalf("Range[%d].Key = %q, want %q", i, got[i].Key, k)
		}
	}

	// Full span covers everything.
	all, err := m.Range("", "zzz")
	if err != nil || len(all) != len(keys) {
		t.Fatalf("full Range = %d entries, %v; want %d", len(all), err, len(keys))
	}

	// The result is an independent copy: mutating it must not leak back.
	got[0].Value = "corrupted"
	if v, _ := m.Get("banana"); v != "v1" {
		t.Fatalf("Range result mutation leaked into map, Get(banana) = %q", v)
	}
}

func TestBoundaryParams(t *testing.T) {
	m := New()
	if err := m.Put("", "v"); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("Put empty key = %v, want ErrEmptyKey", err)
	}
	if ok, err := m.Delete(""); !errors.Is(err, ErrEmptyKey) || ok {
		t.Fatalf("Delete empty key = %v, %v; want false, ErrEmptyKey", ok, err)
	}
	mustPut(t, m, "b", "2")

	if _, err := m.Range("b", "b"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Range(b, b) = %v, want ErrInvalidRange", err)
	}
	if _, err := m.Range("c", "b"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Range(c, b) = %v, want ErrInvalidRange", err)
	}
	// A valid but empty range is not an error.
	got, err := m.Range("a", "b")
	if err != nil || len(got) != 0 {
		t.Fatalf("Range(a, b) = %v, %v; want empty, nil", got, err)
	}

	if ok, err := m.Delete("missing"); err != nil || ok {
		t.Fatalf("Delete(missing) = %v, %v; want false, nil", ok, err)
	}
	if _, ok := m.Get("missing"); ok {
		t.Fatal("Get(missing) reported found")
	}
}

func TestDeleteAcrossLevels(t *testing.T) {
	m := New()
	// Deterministic heights: find a key whose node reaches at least level 3.
	tall := ""
	for i := 0; ; i++ {
		k := fmt.Sprintf("tall-%d", i)
		if keyLevel(k) >= 3 {
			tall = k
			break
		}
	}
	for i := 0; i < 40; i++ {
		mustPut(t, m, fmt.Sprintf("key-%03d", i), "v")
	}
	mustPut(t, m, tall, "tall-value")
	if m.level < 3 {
		t.Fatalf("list height = %d, want >= 3", m.level)
	}
	assertLevelInvariants(t, m)

	ok, err := m.Delete(tall)
	if err != nil || !ok {
		t.Fatalf("Delete(%q) = %v, %v; want true, nil", tall, ok, err)
	}
	if _, found := m.Get(tall); found {
		t.Fatalf("Get(%q) still found after Delete", tall)
	}
	// No dangling index entries may remain at any level.
	assertLevelInvariants(t, m)
	if got := m.Len(); got != 40 {
		t.Fatalf("Len() = %d, want 40", got)
	}

	// Deleting again reports not-found; inserting afterwards still works.
	if ok, err := m.Delete(tall); err != nil || ok {
		t.Fatalf("second Delete(%q) = %v, %v; want false, nil", tall, ok, err)
	}
	mustPut(t, m, "zzz-after", "v")
	assertLevelInvariants(t, m)
}

func TestSnapshotAndClose(t *testing.T) {
	m := New()
	mustPut(t, m, "a", "1")
	mustPut(t, m, "b", "2")
	mustPut(t, m, "c", "3")

	snap := m.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("Snapshot() has %d entries, want 3", len(snap))
	}
	snap[0].Value = "corrupted" // independent copy: must not affect the map
	if v, _ := m.Get("a"); v != "1" {
		t.Fatalf("snapshot mutation leaked into map, Get(a) = %q", v)
	}

	m.Close()
	m.Close() // idempotent

	if err := m.Put("d", "4"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Put after Close = %v, want ErrClosed", err)
	}
	if ok, err := m.Delete("a"); !errors.Is(err, ErrClosed) || ok {
		t.Fatalf("Delete after Close = %v, %v; want false, ErrClosed", ok, err)
	}

	// Reads keep working after Close.
	if v, ok := m.Get("b"); !ok || v != "2" {
		t.Fatalf("Get after Close = %q, %v; want 2, true", v, ok)
	}
	if e, ok := m.LowerBound("b"); !ok || e.Key != "b" {
		t.Fatalf("LowerBound after Close = %+v, %v; want b, true", e, ok)
	}
	if entries, err := m.Range("a", "c"); err != nil || len(entries) != 2 {
		t.Fatalf("Range after Close = %v, %v; want 2 entries", entries, err)
	}
	if got := m.Len(); got != 3 {
		t.Fatalf("Len after Close = %d, want 3", got)
	}
	assertSnapshot(t, m, []Entry{{"a", "1"}, {"b", "2"}, {"c", "3"}})
}

func TestConcurrent(t *testing.T) {
	m := New()
	const workers = 8
	const perWorker = 50

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				key := fmt.Sprintf("key-%02d-%03d", w, i)
				if err := m.Put(key, fmt.Sprintf("v%d", i)); err != nil {
					t.Errorf("Put(%q): %v", key, err)
				}
				if _, ok := m.Get(key); !ok {
					t.Errorf("Get(%q) right after Put failed", key)
				}
				m.LowerBound(key)
				m.Len()
			}
		}(w)
	}
	wg.Wait()

	if got, want := m.Len(), workers*perWorker; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	snap := m.Snapshot()
	if len(snap) != workers*perWorker {
		t.Fatalf("Snapshot() has %d entries, want %d", len(snap), workers*perWorker)
	}
	for i := 1; i < len(snap); i++ {
		if snap[i-1].Key >= snap[i].Key {
			t.Fatalf("snapshot not strictly sorted at %d: %q >= %q", i, snap[i-1].Key, snap[i].Key)
		}
	}
	assertLevelInvariants(t, m)

	// Concurrently delete half the keys while others read.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i += 2 {
				key := fmt.Sprintf("key-%02d-%03d", w, i)
				if ok, err := m.Delete(key); err != nil || !ok {
					t.Errorf("Delete(%q) = %v, %v; want true, nil", key, ok, err)
				}
				m.Snapshot()
			}
		}(w)
	}
	wg.Wait()

	if got, want := m.Len(), workers*perWorker/2; got != want {
		t.Fatalf("Len() after deletes = %d, want %d", got, want)
	}
	assertLevelInvariants(t, m)
}
