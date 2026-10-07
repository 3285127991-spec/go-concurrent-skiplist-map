// Package skiplistmap implements a concurrent sorted string map backed by a
// multi-level skip list. Keys are unique, non-empty strings kept in
// lexicographic order.
package skiplistmap

import (
	"errors"
	"hash/fnv"
	"sync"
)

// maxLevel is the maximum number of forward pointers a node may have.
// With p = 1/2 per extra level this supports maps far beyond any practical size.
const maxLevel = 32

var (
	// ErrEmptyKey is returned when an empty key is supplied to Put.
	ErrEmptyKey = errors.New("skiplistmap: key must not be empty")
	// ErrClosed is returned by Put when the map has been closed.
	ErrClosed = errors.New("skiplistmap: map is closed")
	// ErrInvalidRange is returned by Range when start >= end.
	ErrInvalidRange = errors.New("skiplistmap: range requires start < end")
)

// Entry is a single key/value pair stored in the map.
type Entry struct {
	Key   string
	Value string
}

// node is a skip list node. forward[i] is the next node at level i;
// len(forward) is the node's height.
type node struct {
	key     string
	value   string
	forward []*node
}

// Map is a concurrent skip list map. The zero value is not usable; use New.
type Map struct {
	mu     sync.RWMutex
	head   *node // sentinel, key/value unused
	level  int   // highest level currently in use (1..maxLevel)
	length int
	closed bool
}

// New returns an empty, open Map.
func New() *Map {
	return &Map{
		head:  &node{forward: make([]*node, maxLevel)},
		level: 1,
	}
}

// levelForKey derives a node's height deterministically from its key using
// FNV-1a: trailing zero bits of the hash give a geometric (p = 1/2) height
// distribution, so the same key always gets the same height.
func levelForKey(key string) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	bits := h.Sum64()
	level := 1
	for bits&1 == 0 && level < maxLevel {
		level++
		bits >>= 1
	}
	return level
}

// findGreaterOrEqual returns the first node with key >= target, or nil.
// If prevs is non-nil it is filled with the last node < target at every level.
// Callers must hold at least a read lock.
func (m *Map) findGreaterOrEqual(target string, prevs []*node) *node {
	cur := m.head
	for i := m.level - 1; i >= 0; i-- {
		for cur.forward[i] != nil && cur.forward[i].key < target {
			cur = cur.forward[i]
		}
		if prevs != nil {
			prevs[i] = cur
		}
	}
	return cur.forward[0]
}

// Put inserts key with value, or updates value if key already exists.
// It returns ErrEmptyKey for an empty key and ErrClosed after Close.
func (m *Map) Put(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}

	prevs := make([]*node, maxLevel)
	cur := m.findGreaterOrEqual(key, prevs)
	if cur != nil && cur.key == key {
		cur.value = value
		return nil
	}

	lvl := levelForKey(key)
	if lvl > m.level {
		for i := m.level; i < lvl; i++ {
			prevs[i] = m.head
		}
		m.level = lvl
	}
	nn := &node{key: key, value: value, forward: make([]*node, lvl)}
	for i := 0; i < lvl; i++ {
		nn.forward[i] = prevs[i].forward[i]
		prevs[i].forward[i] = nn
	}
	m.length++
	return nil
}

// Get returns the value stored under key and whether it was found.
func (m *Map) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cur := m.findGreaterOrEqual(key, nil)
	if cur != nil && cur.key == key {
		return cur.value, true
	}
	return "", false
}

// Delete removes key and unlinks its node from every level it appears in.
// It reports whether the key was present. After Close it is a no-op
// returning false.
func (m *Map) Delete(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}

	prevs := make([]*node, maxLevel)
	cur := m.findGreaterOrEqual(key, prevs)
	if cur == nil || cur.key != key {
		return false
	}
	for i := 0; i < len(cur.forward); i++ {
		prevs[i].forward[i] = cur.forward[i]
	}
	for m.level > 1 && m.head.forward[m.level-1] == nil {
		m.level--
	}
	m.length--
	return true
}

// LowerBound returns the first entry whose key is >= key. The second return
// value is false when no such entry exists.
func (m *Map) LowerBound(key string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cur := m.findGreaterOrEqual(key, nil)
	if cur == nil {
		return Entry{}, false
	}
	return Entry{Key: cur.key, Value: cur.value}, true
}

// Range returns an ordered, independent copy of all entries with
// start <= key < end. It requires start < end.
func (m *Map) Range(start, end string) ([]Entry, error) {
	if start >= end {
		return nil, ErrInvalidRange
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Entry
	for cur := m.findGreaterOrEqual(start, nil); cur != nil && cur.key < end; cur = cur.forward[0] {
		out = append(out, Entry{Key: cur.key, Value: cur.value})
	}
	return out, nil
}

// Snapshot returns an ordered, independent copy of every entry in the map.
func (m *Map) Snapshot() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Entry, 0, m.length)
	for cur := m.head.forward[0]; cur != nil; cur = cur.forward[0] {
		out = append(out, Entry{Key: cur.key, Value: cur.value})
	}
	return out
}

// Len returns the number of keys currently stored.
func (m *Map) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.length
}

// Close marks the map closed: subsequent Put calls fail with ErrClosed and
// Delete becomes a no-op, while all read operations keep working.
// Close is idempotent.
func (m *Map) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}
