// Package badgestate owns the GTK-independent update-count bookkeeping used
// by the navigation badge.
package badgestate

import "sync"

// Source identifies one update provider.
type Source uint8

const (
	Bootc Source = iota
	Flatpak
	Homebrew
	sourceCount
)

// Snapshot is the atomic result of changing one provider's count.
type Snapshot struct {
	Count int
	Total int
}

// Counts stores update counts by provider. Its methods are safe for the
// independent bootc, Flatpak, and Homebrew worker goroutines to call.
type Counts struct {
	mu     sync.Mutex
	values [sourceCount]int
}

// Set replaces one provider's count and returns that count plus the new total.
// Negative counts are clamped to zero.
func (c *Counts) Set(source Source, count int) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	if valid(source) {
		c.values[source] = max(0, count)
	}
	return c.snapshot(source)
}

// Add changes one provider's count and returns that count plus the new total.
// The stored count never falls below zero.
func (c *Counts) Add(source Source, delta int) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	if valid(source) {
		c.values[source] = max(0, c.values[source]+delta)
	}
	return c.snapshot(source)
}

// Get returns one provider's current count.
func (c *Counts) Get(source Source) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !valid(source) {
		return 0
	}
	return c.values[source]
}

// Total returns the sum of every provider's current count.
func (c *Counts) Total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total()
}

func (c *Counts) snapshot(source Source) Snapshot {
	value := 0
	if valid(source) {
		value = c.values[source]
	}
	return Snapshot{Count: value, Total: c.total()}
}

func (c *Counts) total() int {
	total := 0
	for _, count := range c.values {
		total += count
	}
	return total
}

func valid(source Source) bool {
	return source < sourceCount
}
