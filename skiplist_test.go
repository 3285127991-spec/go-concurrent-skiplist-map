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
		t.Fatalf("Put(%q) failed: %v", key, err)
	}
}

func assertSnapshotKeys(t *testing.T, m *Map, want []string) {
	t.Helper()
	snap := m.Snapshot()
	if len(snap) != len(want) {
		t.Fatalf("Snapshot has %d entries, want %d: %v", len(snap), len(want), snap)
	}
	for i, key := range want {
		if snap[i].Key != key {
			t.Fatalf("Snapshot[%d].Key = %q, want %q (full: %v)", i, snap[i].Key, key, snap)
		}
	}
}

func TestPutGetAndOrder(t *testing.T) {
	m := New()
	mustPut(t, m, "pear", "1")
	mustPut(t, m, "apple", "2")
	mustPut(t, m, "banana", "3")
	mustPut(t, m, "apricot", "4")

	if got := m.Len(); got != 4 {
		t.Fatalf("Len = %d, want 4", got)
	}
	if v, ok := m.Get("banana"); !ok || v != "3" {
		t.Fatalf("Get(banana) = %q, %v, want \"3\", true", v, ok)
	}
	if _, ok := m.Get("cherry"); ok {
		t.Fatal("Get(cherry) found a missing key")
	}
	assertSnapshotKeys(t, m, []string{"apple", "apricot", "banana", "pear"})
}

func TestPutUpdatesExistingValue(t *testing.T) {
	m := New()
	mustPut(t, m, "k", "v1")
	mustPut(t, m, "k", "v2")

	if got := m.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1: duplicate key inserted", got)
	}
	if v, ok := m.Get("k"); !ok || v != "v2" {
		t.Fatalf("Get(k) = %q, %v, want \"v2\", true", v, ok)
	}
}

func TestLowerBound(t *testing.T) {
	m := New()
	if _, ok := m.LowerBound("a"); ok {
		t.Fatal("LowerBound on empty map reported a result")
	}
	for _, k := range []string{"b", "d", "f"} {
		mustPut(t, m, k, "v-"+k)
	}

	tests := []struct {
		target, wantKey string
		ok              bool
	}{
		{"a", "b", true}, // before all keys
		{"b", "b", true}, // exact match
		{"c", "d", true}, // between keys
		{"f", "f", true}, // last key
		{"g", "", false}, // beyond all keys
	}
	for _, tc := range tests {
		e, ok := m.LowerBound(tc.target)
		if ok != tc.ok {
			t.Fatalf("LowerBound(%q) ok = %v, want %v", tc.target, ok, tc.ok)
		}
		if ok && (e.Key != tc.wantKey || e.Value != "v-"+tc.wantKey) {
			t.Fatalf("LowerBound(%q) = %+v, want key %q", tc.target, e, tc.wantKey)
		}
	}
}

func TestRange(t *testing.T) {
	m := New()
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		mustPut(t, m, k, "v-"+k)
	}

	got, err := m.Range("b", "e")
	if err != nil {
		t.Fatalf("Range failed: %v", err)
	}
	want := []string{"b", "c", "d"} // [start, end): end excluded
	if len(got) != len(want) {
		t.Fatalf("Range returned %d entries, want %d: %v", len(got), len(want), got)
	}
	for i, key := range want {
		if got[i].Key != key {
			t.Fatalf("Range[%d].Key = %q, want %q", i, got[i].Key, key)
		}
	}

	got[0].Value = "mutated" // returned slice must be an independent copy
	if v, _ := m.Get("b"); v != "v-b" {
		t.Fatalf("mutating Range result changed the map: Get(b) = %q", v)
	}

	full, err := m.Range("a", "zzz")
	if err != nil || len(full) != 5 {
		t.Fatalf("Range over all keys = %v, %v", full, err)
	}
	empty, err := m.Range("c", "d")
	if err != nil || len(empty) != 1 || empty[0].Key != "c" {
		t.Fatalf("Range(c,d) = %v, %v", empty, err)
	}
}

// tallKey returns the first candidate whose deterministic level is >= minLevel.
func tallKey(t *testing.T, minLevel int) string {
	t.Helper()
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("tall-%03d", i)
		if levelForKey(key) >= minLevel {
			return key
		}
	}
	t.Fatalf("no candidate key with level >= %d found", minLevel)
	return ""
}

func TestDeleteUnlinksAllLevels(t *testing.T) {
	m := New()
	tall := tallKey(t, 3)
	if lvl := levelForKey(tall); lvl < 3 {
		t.Fatalf("%q has level %d, want >= 3", tall, lvl)
	}

	keys := []string{"aa", "ab", "ac", tall, "ad", "ae"}
	for _, k := range keys {
		mustPut(t, m, k, "v-"+k)
	}
	if !m.Delete(tall) {
		t.Fatalf("Delete(%q) = false, want true", tall)
	}
	if m.Delete(tall) {
		t.Fatalf("second Delete(%q) = true, want false", tall)
	}
	if _, ok := m.Get(tall); ok {
		t.Fatalf("Get(%q) found deleted key", tall)
	}

	// No level may still reference the deleted node (no dangling index links).
	for lvl := 0; lvl < m.level; lvl++ {
		for cur := m.head.forward[lvl]; cur != nil; cur = cur.forward[lvl] {
			if cur.key == tall {
				t.Fatalf("deleted key %q still linked at level %d", tall, lvl)
			}
		}
	}
	// Level-0 list stays sorted and complete.
	assertSnapshotKeys(t, m, []string{"aa", "ab", "ac", "ad", "ae"})
	if e, ok := m.LowerBound("ac"); !ok || e.Key != "ac" {
		t.Fatalf("LowerBound(ac) after delete = %+v, %v", e, ok)
	}
	// Re-inserting the same key must rebuild its index links.
	mustPut(t, m, tall, "v-new")
	if v, ok := m.Get(tall); !ok || v != "v-new" {
		t.Fatalf("Get(%q) after re-put = %q, %v", tall, v, ok)
	}
	if got := m.Len(); got != 6 {
		t.Fatalf("Len = %d, want 6", got)
	}
}

func TestBoundaryAndInvalidParams(t *testing.T) {
	m := New()
	if err := m.Put("", "v"); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("Put empty key err = %v, want ErrEmptyKey", err)
	}
	if _, err := m.Range("b", "a"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Range(b,a) err = %v, want ErrInvalidRange", err)
	}
	if _, err := m.Range("a", "a"); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Range(a,a) err = %v, want ErrInvalidRange", err)
	}
	if m.Delete("missing") {
		t.Fatal("Delete of missing key = true, want false")
	}
	if _, ok := m.Get(""); ok {
		t.Fatal("Get of empty key reported found")
	}
}

func TestSnapshotAndClose(t *testing.T) {
	m := New()
	mustPut(t, m, "x", "1")
	mustPut(t, m, "y", "2")

	snap := m.Snapshot()
	snap[0].Value = "mutated" // snapshot must be an independent copy
	if v, _ := m.Get("x"); v != "1" {
		t.Fatalf("mutating Snapshot changed the map: Get(x) = %q", v)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	if err := m.Put("z", "3"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Put after Close err = %v, want ErrClosed", err)
	}
	if m.Delete("x") {
		t.Fatal("Delete after Close = true, want false")
	}
	// Reads keep working after Close.
	if v, ok := m.Get("x"); !ok || v != "1" {
		t.Fatalf("Get after Close = %q, %v", v, ok)
	}
	if e, ok := m.LowerBound("x"); !ok || e.Key != "x" {
		t.Fatalf("LowerBound after Close = %+v, %v", e, ok)
	}
	if got, err := m.Range("x", "zzz"); err != nil || len(got) != 2 {
		t.Fatalf("Range after Close = %v, %v", got, err)
	}
	assertSnapshotKeys(t, m, []string{"x", "y"})
	if got := m.Len(); got != 2 {
		t.Fatalf("Len after Close = %d, want 2", got)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	m := New()
	const writers = 4
	const perWriter = 25
	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				key := fmt.Sprintf("w%d-key%03d", id, i)
				if err := m.Put(key, fmt.Sprintf("v%d", i)); err != nil {
					t.Errorf("Put(%q) failed: %v", key, err)
				}
				m.Get(key)
				m.LowerBound(key)
			}
			// Delete a few of our own keys while others still write.
			for i := 0; i < 5; i++ {
				m.Delete(fmt.Sprintf("w%d-key%03d", id, i))
			}
		}(w)
	}
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				snap := m.Snapshot()
				for j := 1; j < len(snap); j++ {
					if snap[j-1].Key >= snap[j].Key {
						t.Errorf("snapshot not strictly sorted: %q >= %q", snap[j-1].Key, snap[j].Key)
					}
				}
				m.Len()
			}
		}()
	}
	wg.Wait()

	want := writers * (perWriter - 5)
	if got := m.Len(); got != want {
		t.Fatalf("Len = %d, want %d", got, want)
	}
	// Final state: strictly sorted, unique, and every surviving key readable.
	snap := m.Snapshot()
	if len(snap) != want {
		t.Fatalf("Snapshot has %d entries, want %d", len(snap), want)
	}
	for i, e := range snap {
		if i > 0 && snap[i-1].Key >= e.Key {
			t.Fatalf("final snapshot not strictly sorted at %d: %q >= %q", i, snap[i-1].Key, e.Key)
		}
		if _, ok := m.Get(e.Key); !ok {
			t.Fatalf("Get(%q) missing after concurrent run", e.Key)
		}
	}
}
