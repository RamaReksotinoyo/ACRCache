package main

import (
	"encoding/gob"
	"fmt"
	"os"
)

const defaultStoreFile = ".arccache_store"

// persistEntry is a single serializable cache entry.
type persistEntry struct {
	Key   string
	Value string
}

// saveStore persists the current live entries (T1 + T2) to a file.
func saveStore(cache *ARCCache, path string) error {
	keys := cache.Keys()

	entries := make([]persistEntry, 0, len(keys))
	for _, key := range keys {
		val, ok := cache.Get(key)
		if !ok {
			continue
		}
		str, ok := val.(string)
		if !ok {
			continue
		}
		entries = append(entries, persistEntry{Key: key, Value: str})
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not create store file: %w", err)
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	if err := enc.Encode(entries); err != nil {
		return fmt.Errorf("could not encode store: %w", err)
	}
	return nil
}

// loadStore reads persisted entries from a file and populates the cache.
func loadStore(cache *ARCCache, path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no store yet, that is fine
		}
		return fmt.Errorf("could not open store file: %w", err)
	}
	defer f.Close()

	var entries []persistEntry
	dec := gob.NewDecoder(f)
	if err := dec.Decode(&entries); err != nil {
		return fmt.Errorf("could not decode store: %w", err)
	}

	for _, e := range entries {
		cache.Put(e.Key, e.Value)
	}
	return nil
}
