// Command skipcli demonstrates the skiplistmap library: insert, update,
// LowerBound, Range, Delete, and reading after Close.
package main

import (
	"fmt"
	"os"

	"skiplistmap"
)

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func main() {
	m := skiplistmap.New()

	fmt.Println("== Put: insert 5 entries ==")
	for _, e := range []skiplistmap.Entry{
		{Key: "banana", Value: "yellow fruit"},
		{Key: "apple", Value: "red fruit"},
		{Key: "cherry", Value: "small red fruit"},
		{Key: "apricot", Value: "orange fruit"},
		{Key: "date", Value: "sweet brown fruit"},
	} {
		check(m.Put(e.Key, e.Value))
	}
	fmt.Println("Len:", m.Len())

	fmt.Println("\n== Put: update existing key (no new node) ==")
	check(m.Put("apple", "green fruit"))
	if v, ok := m.Get("apple"); ok {
		fmt.Printf("Get(apple) = %q\n", v)
	}
	fmt.Println("Len:", m.Len())

	fmt.Println("\n== Snapshot: lexicographic order ==")
	for _, e := range m.Snapshot() {
		fmt.Printf("  %-8s -> %s\n", e.Key, e.Value)
	}

	fmt.Println("\n== LowerBound ==")
	for _, target := range []string{"app", "banana", "zzz"} {
		if e, ok := m.LowerBound(target); ok {
			fmt.Printf("LowerBound(%q) = %q -> %s\n", target, e.Key, e.Value)
		} else {
			fmt.Printf("LowerBound(%q) = not found\n", target)
		}
	}

	fmt.Println("\n== Range [apricot, cherry) ==")
	entries, err := m.Range("apricot", "cherry")
	check(err)
	for _, e := range entries {
		fmt.Printf("  %-8s -> %s\n", e.Key, e.Value)
	}

	fmt.Println("\n== Delete ==")
	fmt.Println("Delete(banana):", m.Delete("banana"))
	fmt.Println("Delete(banana) again:", m.Delete("banana"))
	fmt.Println("Len:", m.Len())

	fmt.Println("\n== Close: writes rejected, reads still work ==")
	check(m.Close())
	check(m.Close()) // idempotent
	if err := m.Put("fig", "purple fruit"); err != nil {
		fmt.Println("Put after Close:", err)
	}
	fmt.Println("Delete(apple) after Close:", m.Delete("apple"))
	if v, ok := m.Get("apple"); ok {
		fmt.Printf("Get(apple) after Close = %q\n", v)
	}
	fmt.Println("Len after Close:", m.Len())
	fmt.Println("Snapshot after Close:")
	for _, e := range m.Snapshot() {
		fmt.Printf("  %-8s -> %s\n", e.Key, e.Value)
	}
}
