package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultCapacity = 128

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	switch args[0] {
	case "get":
		cmdGet(args[1:])
	case "set":
		cmdSet(args[1:])
	case "delete", "del":
		cmdDelete(args[1:])
	case "keys":
		cmdKeys(args[1:])
	case "stats":
		cmdStats(args[1:])
	case "flush":
		cmdFlush(args[1:])
	case "bench":
		cmdBench(args[1:])
	case "repl":
		cmdREPL(args[1:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\nRun 'arccache help' for usage.\n", args[0])
		os.Exit(1)
	}
}

// cmdGet retrieves a value from the persistent store.
func cmdGet(args []string) {
	if len(args) < 1 {
		die("usage: arccache get <key>")
	}
	key := args[0]

	cache := NewARCCache(defaultCapacity)
	if err := loadStore(cache, defaultStoreFile); err != nil {
		die("failed to load store: %v", err)
	}

	val, ok := cache.Get(key)
	if !ok {
		fmt.Fprintf(os.Stderr, "(nil)\n")
		os.Exit(1)
	}
	fmt.Println(val)

	// Re-save so that the access order (T1→T2 promotion) is persisted
	if err := saveStore(cache, defaultStoreFile); err != nil {
		die("failed to save store: %v", err)
	}
}

// cmdSet stores a key-value pair.
func cmdSet(args []string) {
	if len(args) < 2 {
		die("usage: arccache set <key> <value>")
	}
	key := args[0]
	value := strings.Join(args[1:], " ")

	cache := NewARCCache(defaultCapacity)
	if err := loadStore(cache, defaultStoreFile); err != nil {
		die("failed to load store: %v", err)
	}

	cache.Put(key, value)

	if err := saveStore(cache, defaultStoreFile); err != nil {
		die("failed to save store: %v", err)
	}
	fmt.Println("OK")
}

// cmdDelete removes a key from the store.
func cmdDelete(args []string) {
	if len(args) < 1 {
		die("usage: arccache delete <key>")
	}
	key := args[0]

	cache := NewARCCache(defaultCapacity)
	if err := loadStore(cache, defaultStoreFile); err != nil {
		die("failed to load store: %v", err)
	}

	if cache.Delete(key) {
		if err := saveStore(cache, defaultStoreFile); err != nil {
			die("failed to save store: %v", err)
		}
		fmt.Println("OK")
	} else {
		fmt.Fprintf(os.Stderr, "(nil) key not found\n")
		os.Exit(1)
	}
}

// cmdKeys lists all live keys in the cache.
func cmdKeys(args []string) {
	_ = args

	cache := NewARCCache(defaultCapacity)
	if err := loadStore(cache, defaultStoreFile); err != nil {
		die("failed to load store: %v", err)
	}

	keys := cache.Keys()
	if len(keys) == 0 {
		fmt.Println("(empty)")
		return
	}
	for i, k := range keys {
		fmt.Printf("%d) %s\n", i+1, k)
	}
}

// cmdStats prints detailed ARC internal state.
func cmdStats(args []string) {
	_ = args

	cache := NewARCCache(defaultCapacity)
	if err := loadStore(cache, defaultStoreFile); err != nil {
		die("failed to load store: %v", err)
	}

	// Replay one full pass to get meaningful stats
	stats := cache.Stats()

	fmt.Println()
	fmt.Printf("  ARC Cache Stats\n")
	fmt.Printf("  %s\n", strings.Repeat("─", 36))
	fmt.Printf("  Capacity  : %d\n", defaultCapacity)
	fmt.Printf("  p (target): %d\n", stats.P)
	fmt.Println()
	fmt.Printf("  Live entries\n")
	fmt.Printf("    T1 (seen once) : %d\n", stats.T1Size)
	fmt.Printf("    T2 (seen 2x+)  : %d\n", stats.T2Size)
	fmt.Println()
	fmt.Printf("  Ghost entries\n")
	fmt.Printf("    B1 (evicted T1): %d\n", stats.B1Size)
	fmt.Printf("    B2 (evicted T2): %d\n", stats.B2Size)
	fmt.Println()
	fmt.Printf("  Hits      : %d\n", stats.Hits)
	fmt.Printf("  Misses    : %d\n", stats.Misses)
	fmt.Printf("  Evictions : %d\n", stats.Evictions)
	fmt.Printf("  Hit rate  : %.1f%%\n", stats.HitRate())
	fmt.Println()

	// Visual bar for T1 vs T2 balance
	total := stats.T1Size + stats.T2Size
	if total > 0 {
		barWidth := 32
		t1Filled := stats.T1Size * barWidth / total
		t2Filled := barWidth - t1Filled
		fmt.Printf("  T1/T2 balance:\n")
		fmt.Printf("  T1 |%s%s| T2\n",
			strings.Repeat("█", t1Filled),
			strings.Repeat("░", t2Filled))
		fmt.Println()
	}
}

// cmdFlush clears the entire cache and store file.
func cmdFlush(args []string) {
	_ = args

	if err := os.Remove(defaultStoreFile); err != nil && !os.IsNotExist(err) {
		die("failed to flush store: %v", err)
	}
	fmt.Println("OK (store cleared)")
}

// cmdBench runs the benchmark suite.
func cmdBench(args []string) {
	capacity := 64
	ops := 10000

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--capacity", "-c":
			if i+1 < len(args) {
				i++
				v, err := strconv.Atoi(args[i])
				if err != nil || v < 1 {
					die("invalid capacity: %s", args[i])
				}
				capacity = v
			}
		case "--ops", "-o":
			if i+1 < len(args) {
				i++
				v, err := strconv.Atoi(args[i])
				if err != nil || v < 1 {
					die("invalid ops: %s", args[i])
				}
				ops = v
			}
		}
	}

	runBench(capacity, ops)
}

// cmdREPL starts an interactive REPL session.
func cmdREPL(args []string) {
	_ = args

	cache := NewARCCache(defaultCapacity)
	if err := loadStore(cache, defaultStoreFile); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load store: %v\n", err)
	}

	fmt.Println("arccache REPL — type 'help' for commands, 'exit' to quit")
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := parts[0]
		cmdArgs := parts[1:]

		switch cmd {
		case "exit", "quit", "q":
			if err := saveStore(cache, defaultStoreFile); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to save store: %v\n", err)
			}
			fmt.Println("bye")
			return

		case "set":
			if len(cmdArgs) < 2 {
				fmt.Println("usage: set <key> <value>")
				continue
			}
			cache.Put(cmdArgs[0], strings.Join(cmdArgs[1:], " "))
			fmt.Println("OK")

		case "get":
			if len(cmdArgs) < 1 {
				fmt.Println("usage: get <key>")
				continue
			}
			val, ok := cache.Get(cmdArgs[0])
			if !ok {
				fmt.Println("(nil)")
			} else {
				fmt.Println(val)
			}

		case "del", "delete":
			if len(cmdArgs) < 1 {
				fmt.Println("usage: del <key>")
				continue
			}
			if cache.Delete(cmdArgs[0]) {
				fmt.Println("OK")
			} else {
				fmt.Println("(nil) key not found")
			}

		case "keys":
			keys := cache.Keys()
			if len(keys) == 0 {
				fmt.Println("(empty)")
			} else {
				for i, k := range keys {
					fmt.Printf("%d) %s\n", i+1, k)
				}
			}

		case "stats":
			stats := cache.Stats()
			fmt.Printf("p=%d  T1=%d  T2=%d  B1=%d  B2=%d  hits=%d  misses=%d  hit_rate=%.1f%%\n",
				stats.P, stats.T1Size, stats.T2Size, stats.B1Size, stats.B2Size,
				stats.Hits, stats.Misses, stats.HitRate())

		case "flush":
			cache.Flush()
			fmt.Println("OK (cache cleared)")

		case "bench":
			runBench(32, 5000)

		case "help":
			fmt.Println("  set <key> <value>  store a key")
			fmt.Println("  get <key>          retrieve a key")
			fmt.Println("  del <key>          delete a key")
			fmt.Println("  keys               list all keys")
			fmt.Println("  stats              show ARC internals")
			fmt.Println("  flush              clear all entries")
			fmt.Println("  bench              run benchmark")
			fmt.Println("  exit               save and quit")

		default:
			fmt.Printf("unknown command: %s (type 'help' for commands)\n", cmd)
		}
	}
}

// die prints a formatted error message and exits with code 1.
func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func printUsage() {
	fmt.Println(`arccache — a key-value store powered by Adaptive Replacement Cache

Usage:
  arccache <command> [arguments]

Commands:
  set <key> <value>          store a key-value pair
  get <key>                  retrieve a value by key
  delete <key>               remove a key
  keys                       list all live keys
  stats                      show ARC internal state
  flush                      clear the entire store
  bench [--capacity N]       run benchmark (scan/freq/mixed/random)
        [--ops N]
  repl                       start interactive session

Examples:
  arccache set name "Anugrah"
  arccache get name
  arccache stats
  arccache bench --capacity 64 --ops 50000
  arccache repl`)
}
