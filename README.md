# arccache

A CLI key-value store powered by an [Adaptive Replacement Cache (ARC)](https://en.wikipedia.org/wiki/Adaptive_replacement_cache) engine. Built to learn how ARC works by actually using it.

## What is ARC?

ARC is a cache eviction algorithm that adapts between recency (LRU) and frequency (LFU) automatically. It maintains four internal lists:

- **T1** — pages seen exactly once recently
- **T2** — pages seen more than once (promoted from T1)
- **B1 / B2** — ghost entries (keys only, no values) of recently evicted T1/T2 items

A parameter `p` shifts dynamically based on ghost hits — if B1 gets hit, `p` grows (favor recency); if B2 gets hit, `p` shrinks (favor frequency). This is what makes ARC self-tuning.

## Usage

```bash
go build -o arccache .
```

```bash
./arccache set <key> <value>   # store a key
./arccache get <key>           # retrieve a value
./arccache delete <key>        # remove a key
./arccache keys                # list all live keys
./arccache stats               # inspect ARC internals (T1, T2, p, hit rate)
./arccache flush               # clear everything
./arccache bench               # run scan/frequency/mixed/random benchmarks
./arccache repl                # interactive session
```

Data is persisted to `.arccache_store` in the current directory via `encoding/gob`.

## Benchmark

```bash
./arccache bench --capacity 64 --ops 50000
```

Shows hit rate and final `p` value across four workload patterns, plus a bar chart of how `p` moves during a mixed workload — making ARC's adaptation visible.

## Project Structure

```
arc.go        # core ARC implementation (T1/T2/B1/B2, replace, p adaptation)
lru.go        # doubly linked list used by all four ARC lists
store.go      # gob-based persistence
bench.go      # benchmark patterns and p-movement visualization
main.go       # CLI commands
arc_test.go   # unit tests and benchmarks
```

## Running Tests

```bash
go test ./... -v
go test -bench=. -benchmem
```
