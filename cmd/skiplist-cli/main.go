// Command skiplist-cli demonstrates the skiplistmap library: insert,
// update, lower bound, range scan, delete, and reading after Close.
package main

import (
	"fmt"

	"skiplistmap"
)

func main() {
	m := skiplistmap.New()

	fmt.Println("== Put ==")
	for _, kv := range [][2]string{
		{"banana", "yellow fruit"},
		{"apple", "red fruit"},
		{"cherry", "small red fruit"},
		{"apricot", "orange fruit"},
		{"blueberry", "blue berry"},
		{"avocado", "green fruit"},
	} {
		if err := m.Put(kv[0], kv[1]); err != nil {
			fmt.Println("put failed:", err)
			continue
		}
		fmt.Printf("put %-10s -> %s\n", kv[0], kv[1])
	}
	fmt.Println("len:", m.Len())

	fmt.Println("\n== Update existing key ==")
	if err := m.Put("apple", "green fruit"); err != nil {
		fmt.Println("update failed:", err)
	}
	if v, ok := m.Get("apple"); ok {
		fmt.Printf("apple now -> %s (len still %d, no new node)\n", v, m.Len())
	}

	fmt.Println("\n== LowerBound ==")
	if e, ok := m.LowerBound("b"); ok {
		fmt.Printf("lower_bound(%q) = %s -> %s\n", "b", e.Key, e.Value)
	}
	if _, ok := m.LowerBound("zzz"); !ok {
		fmt.Println("lower_bound(\"zzz\") = <not found>")
	}

	fmt.Println("\n== Range [apple, blueberry) ==")
	entries, err := m.Range("apple", "blueberry")
	if err != nil {
		fmt.Println("range failed:", err)
	}
	for _, e := range entries {
		fmt.Printf("  %-10s -> %s\n", e.Key, e.Value)
	}

	fmt.Println("\n== Delete ==")
	if ok, err := m.Delete("banana"); err != nil {
		fmt.Println("delete failed:", err)
	} else {
		fmt.Printf("delete banana: %v (len now %d)\n", ok, m.Len())
	}

	fmt.Println("\n== Snapshot ==")
	for _, e := range m.Snapshot() {
		fmt.Printf("  %-10s -> %s\n", e.Key, e.Value)
	}

	fmt.Println("\n== Close ==")
	m.Close()
	m.Close() // idempotent
	fmt.Println("closed (twice, idempotent)")
	if err := m.Put("mango", "tropical fruit"); err != nil {
		fmt.Println("put after close:  ", err)
	}
	if _, err := m.Delete("apple"); err != nil {
		fmt.Println("delete after close:", err)
	}

	fmt.Println("\n== Reads still work after Close ==")
	if v, ok := m.Get("apple"); ok {
		fmt.Printf("get apple -> %s\n", v)
	}
	if e, ok := m.LowerBound("c"); ok {
		fmt.Printf("lower_bound(%q) = %s -> %s\n", "c", e.Key, e.Value)
	}
	fmt.Printf("len: %d, snapshot entries: %d\n", m.Len(), len(m.Snapshot()))
}
