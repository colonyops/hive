---
name: go-benchmark
description: Write and analyze Go benchmarks using modern patterns and tools
argument-hint: [benchmark-task]
---

# Go Benchmarking - Modern Patterns and Best Practices

Write effective Go benchmarks using the new Loop pattern and analyze results with benchstat.

## Core Pattern: The Loop Method

Use `b.Loop()` instead of explicit `for i := 0; i < b.N; i++` loops:

```go
func BenchmarkExample(b *testing.B) {
    // Setup - not measured
    data := generateTestData()

    for b.Loop() {
        // Code to measure
        result := processData(data)
        _ = result // Keep result alive
    }

    // Cleanup - not measured
}
```

### Why Loop?

- Automatically manages timer (resets on first call, stops after loop)
- Prevents compiler optimizations that would invalidate results
- Disables inlining of functions called within the loop body
- Runs benchmark function once per measurement (vs multiple times with b.N)
- Keeps arguments and results alive to prevent dead code elimination

### Critical Rules

1. **Loop condition must be exactly `b.Loop()`** - variations don't work
2. **Only use Loop OR b.N, never both** - mixing creates incorrect measurements
3. **Optimizations apply only within loop braces** - called functions optimize normally
4. **Setup before loop, cleanup after** - neither counts toward measurement

## Incremental Approach

**Philosophy:** Make small changes, measure each change, understand impact before proceeding.

### Workflow

1. **Baseline first**: Write benchmark for current code
2. **Run multiple times**: `go test -bench=. -count=10 > old.txt`
3. **Make one change**: Modify one aspect of the implementation
4. **Measure again**: `go test -bench=. -count=10 > new.txt`
5. **Compare**: `benchstat old.txt new.txt`
6. **Decide**: Keep change if improvement is significant and stable
7. **Repeat**: Make next change from proven baseline

Never change multiple things at once - you won't know which change caused the impact.

## benchstat - Statistical Analysis

Install: `go install golang.org/x/perf/cmd/benchstat@latest`

### Basic Comparison

```bash
# Run old version
go test -bench=BenchmarkProcess -count=10 > old.txt

# Make changes, run new version
go test -bench=BenchmarkProcess -count=10 > new.txt

# Compare with statistical significance
benchstat old.txt new.txt
```

### Reading benchstat Output

```
name          old time/op    new time/op    delta
Process-8     1.23µs ± 2%    0.98µs ± 1%  -20.33%  (p=0.000 n=10+10)
```

- `±` indicates variance (lower is more stable)
- `delta` shows percentage change
- `p` value indicates statistical significance (< 0.05 is significant)
- `n` shows sample size

**Ignore changes under 5%** - measurement noise is real.

## Best Practices

### Prevent Compiler Optimizations

```go
// BAD: Result not used, entire loop may be optimized away
for b.Loop() {
    processData(data)
}

// GOOD: Assign to package-level var
var result int
for b.Loop() {
    result = processData(data)
}

// GOOD: Use within loop body (Loop keeps it alive)
for b.Loop() {
    r := processData(data)
    _ = r
}
```

### Benchmark Allocations

```go
func BenchmarkAllocations(b *testing.B) {
    b.ReportAllocs() // Include allocation stats in output

    for b.Loop() {
        data := make([]byte, 1024) // Measure this allocation
        _ = data
    }
}
```

Look for `allocs/op` in output - reducing allocations often improves performance more than CPU optimization.

### Sub-benchmarks for Variations

```go
func BenchmarkEncode(b *testing.B) {
    sizes := []int{1, 10, 100, 1000}

    for _, size := range sizes {
        b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
            data := make([]byte, size)
            for b.Loop() {
                encode(data)
            }
        })
    }
}
```

Run specific sub-benchmark: `go test -bench=BenchmarkEncode/size=100`

### Avoid Timer Manipulation

```go
// BAD: Manual timer management is error-prone
for b.Loop() {
    b.StopTimer()
    setup := prepareData()
    b.StartTimer()
    process(setup)
}

// GOOD: Move setup outside loop
for b.Loop() {
    setup := prepareData()
    process(setup)
}

// BEST: If setup must be per-iteration, benchmark it separately
```

Loop handles timer automatically - manual control usually indicates wrong benchmark structure.

### Parallel Benchmarks

```go
func BenchmarkParallel(b *testing.B) {
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() { // Use pb.Next() not b.Loop() in parallel
            doWork()
        }
    })
}
```

Note: `RunParallel` uses `pb.Next()` not `b.Loop()` - different API for concurrent execution.

## Common Patterns

### Benchmarking HTTP Handlers

```go
func BenchmarkHandler(b *testing.B) {
    handler := NewHandler()
    req := httptest.NewRequest("GET", "/api/data", nil)

    for b.Loop() {
        w := httptest.NewRecorder()
        handler.ServeHTTP(w, req)
    }
}
```

### Benchmarking Database Operations

```go
func BenchmarkQuery(b *testing.B) {
    db := setupTestDB(b)
    defer db.Close()

    b.ResetTimer() // Don't measure setup

    for b.Loop() {
        rows, err := db.Query("SELECT * FROM users WHERE age > ?", 18)
        if err != nil {
            b.Fatal(err)
        }
        rows.Close()
    }
}
```

### Table-Driven Benchmarks

```go
func BenchmarkHash(b *testing.B) {
    cases := []struct {
        name string
        size int
    }{
        {"small", 16},
        {"medium", 1024},
        {"large", 65536},
    }

    for _, tc := range cases {
        b.Run(tc.name, func(b *testing.B) {
            data := make([]byte, tc.size)
            for b.Loop() {
                hash(data)
            }
        })
    }
}
```

## Running Benchmarks

```bash
# All benchmarks in package
go test -bench=.

# Specific benchmark
go test -bench=BenchmarkEncode

# With memory allocations
go test -bench=. -benchmem

# Multiple runs for stability
go test -bench=. -count=10

# Longer runs for accuracy
go test -bench=. -benchtime=10s

# CPU profiling
go test -bench=. -cpuprofile=cpu.prof

# Memory profiling
go test -bench=. -memprofile=mem.prof
```

## Analysis Tools

### CPU Profiling

```bash
go test -bench=BenchmarkProcess -cpuprofile=cpu.prof
go tool pprof cpu.prof
# Then: top, list FunctionName, web
```

### Memory Profiling

```bash
go test -bench=BenchmarkProcess -memprofile=mem.prof
go tool pprof -alloc_space mem.prof
```

### Comparing Multiple Changes

```bash
# Run baseline
go test -bench=. -count=10 > baseline.txt

# After change 1
go test -bench=. -count=10 > change1.txt
benchstat baseline.txt change1.txt

# After change 2
go test -bench=. -count=10 > change2.txt
benchstat baseline.txt change2.txt

# Compare all three
benchstat baseline.txt change1.txt change2.txt
```

## Red Flags

**Unstable results (high variance):**
- Increase `-count` to get more samples
- Close other applications
- Check for background tasks affecting CPU
- Consider using `-cpu=1` to reduce scheduling effects

**Results too good to be true:**
- Compiler probably optimized away your code
- Ensure results are used (assign to var or use in assertion)
- Check with `-gcflags='-m'` to see optimization decisions

**Inconsistent improvements:**
- Results under 5% change are often noise
- Need more samples (`-count=20`)
- Consider using `-benchtime=5s` for longer, more stable runs

## Guidelines

1. **Always use b.Loop()** - never explicit `for i := 0; i < b.N` loops
2. **One change at a time** - incremental measurement reveals real impact
3. **Use benchstat** - statistical significance matters more than single runs
4. **Report allocations** - `b.ReportAllocs()` often finds easy wins
5. **Keep results alive** - assign to var or use `_` to prevent dead code elimination
6. **Setup outside loop** - only measure what matters
7. **Run multiple times** - `-count=10` minimum for reliable comparison
8. **Ignore small changes** - under 5% is probably noise
9. **Profile when stuck** - CPU and memory profiles show bottlenecks
10. **Measure before optimizing** - assumptions are usually wrong

## Implementation Checklist

When /go-benchmark is invoked:

1. ✓ Use `b.Loop()` pattern
2. ✓ Add `b.ReportAllocs()` if allocations matter
3. ✓ Keep results alive (assign to var or `_ =`)
4. ✓ Setup before loop, cleanup after
5. ✓ Run with `-count=10` for baseline
6. ✓ Make one incremental change
7. ✓ Measure and compare with `benchstat`
8. ✓ Verify improvement is significant (> 5%, low p-value)
9. ✓ Document findings in commit message
10. ✓ Consider profiling if results unexpected
