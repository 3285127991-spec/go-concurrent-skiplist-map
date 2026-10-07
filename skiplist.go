// Package skiplistmap implements a concurrent-safe ordered map from string
// keys to string values, backed by a multi-level skip list.
//
// Node heights are derived deterministically from each key's FNV-1a hash,
// so a given key always reaches the same height. Lookups, inserts and deletes
// descend the index levels for O(log n) average time; scans run in O(m).
package skiplistmap

import (
	"errors"
	"sync"
)

// maxLevel is the maximum height a node may reach. With a per-level
// branching probability of 1/2 this comfortably covers any realistic size.
const maxLevel = 32

var (
	// ErrEmptyKey is returned when a mutating operation receives an empty key.
	ErrEmptyKey = errors.New("skiplistmap: key must not be empty")
	// ErrClosed is returned by Put and Delete after the map has been closed.
	ErrClosed = errors.New("skiplistmap: map is closed")
	// ErrInvalidRange is returned by Range when start >= end.
	ErrInvalidRange = errors.New("skiplistmap: invalid range, require start < end")
)

// Entry is a single key/value pair stored in the map.
type Entry struct {
	Key   string
	Value string
}

// node is a skip list node; next[i] links to the next node at level i.
type node struct {
	key   string
	value string
	next  []*node
}

// Map is a concurrent-safe ordered string map. Create one with New.
type Map struct {
	mu     sync.RWMutex
	head   *node
	level  int // highest level currently in use, >= 1
	length int
	closed bool
}

// New returns an empty, ready-to-use Map.
func New() *Map {
	return &Map{
		head:  &node{next: make([]*node, maxLevel)},
		level: 1,
	}
}

// fnv32a is the FNV-1a 32-bit hash of s.
func fnv32a(s string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return h
}

// keyLevel derives a node's height deterministically from its key: one plus
// the number of trailing zero bits in the key's hash. The same key always
// maps to the same height, and heights follow a geometric distribution
// with p = 1/2.
func keyLevel(key string) int {
	bits := fnv32a(key)
	level := 1
	for level < maxLevel && bits&1 == 0 {
		level++
		bits >>= 1
	}
	return level
}

// find returns the last node with key < target, walking the index levels
// from top to bottom. When update is non-nil it records that predecessor
// at every level. Callers must hold at least a read lock.
func (m *Map) find(target string, update []*node) *node {
	x := m.head
	for i := m.level - 1; i >= 0; i-- {
		for x.next[i] != nil && x.next[i].key < target {
			x = x.next[i]
		}
		if update != nil {
			update[i] = x
		}
	}
	return x
}

// Put stores value under key. If key already exists only its value is
// replaced; no second node is ever created. It fails with ErrEmptyKey for
// an empty key and ErrClosed after Close.
func (m *Map) Put(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}

	var update [maxLevel]*node
	m.find(key, update[:])
	if next := update[0].next[0]; next != nil && next.key == key {
		next.value = value
		return nil
	}

	lvl := keyLevel(key)
	if lvl > m.level {
		for i := m.level; i < lvl; i++ {
			update[i] = m.head
		}
		m.level = lvl
	}
	x := &node{key: key, value: value, next: make([]*node, lvl)}
	for i := 0; i < lvl; i++ {
		x.next[i] = update[i].next[i]
		update[i].next[i] = x
	}
	m.length++
	return nil
}

// Get returns the value stored under key and whether it was found.
func (m *Map) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	x := m.find(key, nil).next[0]
	if x != nil && x.key == key {
		return x.value, true
	}
	return "", false
}

// Delete removes key, unlinking its node from every level it appears in so
// no dangling index entries remain. It reports whether the key existed and
// fails with ErrClosed after Close.
func (m *Map) Delete(key string) (bool, error) {
	if key == "" {
		return false, ErrEmptyKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false, ErrClosed
	}

	var update [maxLevel]*node
	m.find(key, update[:])
	x := update[0].next[0]
	if x == nil || x.key != key {
		return false, nil
	}
	for i := 0; i < len(x.next); i++ {
		update[i].next[i] = x.next[i]
	}
	for m.level > 1 && m.head.next[m.level-1] == nil {
		m.level--
	}
	m.length--
	return true, nil
}

// LowerBound returns the first entry whose key is >= target. The second
// return value is false when no such entry exists.
func (m *Map) LowerBound(target string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	x := m.find(target, nil).next[0]
	if x == nil {
		return Entry{}, false
	}
	return Entry{Key: x.key, Value: x.value}, true
}

// Range returns an ordered, independent copy of all entries with
// start <= key < end. It requires start < end.
func (m *Map) Range(start, end string) ([]Entry, error) {
	if start >= end {
		return nil, ErrInvalidRange
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Entry, 0)
	for x := m.find(start, nil).next[0]; x != nil && x.key < end; x = x.next[0] {
		out = append(out, Entry{Key: x.key, Value: x.value})
	}
	return out, nil
}

// Snapshot returns an ordered, independent copy of every entry in the map.
func (m *Map) Snapshot() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Entry, 0, m.length)
	for x := m.head.next[0]; x != nil; x = x.next[0] {
		out = append(out, Entry{Key: x.key, Value: x.value})
	}
	return out
}

// Len returns the number of keys currently stored.
func (m *Map) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.length
}

// Close marks the map closed: subsequent Put and Delete calls fail with
// ErrClosed, while Get, LowerBound, Range, Snapshot and Len keep working.
// Close is idempotent.
func (m *Map) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}
