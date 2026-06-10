# Project Conventions

## Language & Toolchain

- All code: **Go** (latest stable)
- Tests: `go test -race -v ./...`
- Linting: `go vet ./...`
- Format: `gofmt` (enforced)
- Module: initialize with `go mod init` at project start

---

## Approach to Any Problem (6-Step Flow)

### Step 1 — Clarify Requirements (2-4 min)
- Separate **functional** (what the system does) from **non-functional** (concurrency, scale, extensibility)
- Ask: what's in scope? What's out?
- Write requirements as comments at the top of the main file before any code

### Step 2 — Identify Core Entities
- Scan requirements for **nouns** → these become structs
- Only model what requirements demand. Don't over-model

### Step 3 — Define Relationships & Responsibilities
- For each pair of entities decide: association, aggregation, or composition
- Each struct gets **one** clear responsibility (SRP)
- Draw the rough structure in comments or a design doc before coding

### Step 4 — Apply Design Patterns Where They Fit
- Patterns are tools, not goals. **Never force a pattern**
- Be ready to justify every pattern choice with the problem signal it solves
- See the pattern selection guide below

### Step 5 — Handle Concurrency & Edge Cases
- Identify shared mutable state. Protect it
- Name the concurrency primitives you'll use and why
- List edge cases explicitly before coding

### Step 6 — Code the Skeleton, Then Walk a Flow
- Write interfaces and key structs first
- Implement one end-to-end flow completely
- Then fill in remaining methods

---

## SOLID Principles (Enforced in All Code)

### S — Single Responsibility
- Each struct has exactly one reason to change
- If a struct name needs "And" to describe it → split it
- Separate: business logic, storage, formatting, coordination

### O — Open/Closed
- Add new behavior via new interface implementations, not by editing existing code
- Use Strategy pattern for interchangeable algorithms
- Use Decorator for layering behavior

### L — Liskov Substitution
- Any interface implementation must be swappable without breaking callers
- Never add methods that some implementations can't fulfill
- If `Square extends Rectangle` feels wrong → don't force inheritance

### I — Interface Segregation
- Keep interfaces small: 1-3 methods
- Clients should not depend on methods they don't use
- Multiple focused interfaces > one fat interface

### D — Dependency Inversion
- High-level modules depend on interfaces, not concrete types
- Inject dependencies through constructors: `func New(dep Interface) *Struct`
- Never instantiate dependencies inside the thing that uses them

---

## OOP in Go (Idiomatic Mapping)

| OOP Concept | Go Idiom |
|-------------|----------|
| Encapsulation | Unexported fields + exported methods |
| Abstraction | Interfaces (behavior contracts) |
| Inheritance | Struct embedding (composition) |
| Polymorphism | Implicit interface satisfaction |

### Rules
- **Composition over inheritance** — always
- **Accept interfaces, return structs**
- Interfaces belong to the **consumer** package, not the provider
- No `interface{}` / `any` unless using generics

---

## Design Pattern Selection Guide

### When to Use What

| Problem Signal | Pattern |
|----------------|---------|
| Interchangeable algorithms / rules | Strategy |
| Notify many on change | Observer |
| Behavior depends on lifecycle state | State |
| Undo / queue / log operations | Command |
| One instance only | Singleton (`sync.Once`) |
| Centralize / parameterize object creation | Factory |
| Too many constructor params / immutable | Builder (functional options in Go) |
| Wrap incompatible API | Adapter |
| Add features dynamically in combos | Decorator |
| Simplify a complex subsystem | Facade |
| Tree of part-whole objects | Composite |
| Control access / lazy-load / cache | Proxy |
| Pass request through handlers | Chain of Responsibility |
| Decouple abstraction from implementation | Bridge |
| Share common state across many objects | Flyweight |
| Fixed process, varying steps | Template Method |

### Go-Specific Concurrency Patterns

| Pattern | When |
|---------|------|
| Mutex + Cond | Protecting shared state with wait/signal |
| Channels | Communication between goroutines |
| sync.Once | Lazy singleton initialization |
| sync.Pool | Reusable temporary objects |
| Semaphore (buffered chan) | Bounding concurrent access |
| Fan-out / Fan-in | Parallel work distribution + collection |
| Worker Pool | Bounded goroutines processing a queue |
| Circuit Breaker | Preventing cascade failures |
| Context cancellation | Propagating deadlines / shutdown |

---

## Concurrency Checklist

- [ ] Identify ALL shared mutable state
- [ ] Choose primitive: Mutex (state protection) vs Channel (communication)
- [ ] Every goroutine has a clear owner (who starts it, who stops it)
- [ ] Every goroutine has a shutdown path (context, done channel, or WaitGroup)
- [ ] No naked goroutines — always `defer wg.Done()` or select on `ctx.Done()`
- [ ] Use `sync.Mutex` for struct-level state; `sync.RWMutex` when reads >> writes
- [ ] Use `atomic` for simple counters/flags
- [ ] Test with `-race` flag always

---

## Deep Concurrency Patterns

### errgroup — Structured Goroutine Lifecycle

Use `golang.org/x/sync/errgroup` when you need to run N goroutines and fail fast on first error.

```go
g, ctx := errgroup.WithContext(parentCtx)

for _, item := range items {
    item := item // capture loop var
    g.Go(func() error {
        return process(ctx, item)
    })
}

if err := g.Wait(); err != nil {
    // first error from any goroutine
}
```

**When to use:** parallel HTTP calls, batch processing, fan-out where any failure aborts all.
**Key rule:** always use the derived `ctx` inside goroutines — it cancels on first error.

### Pipeline Pattern — Stage-Based Processing

Chain goroutines via channels where each stage does one transformation.

```go
func generate(ctx context.Context, nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            select {
            case out <- n:
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}

func square(ctx context.Context, in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for n := range in {
            select {
            case out <- n * n:
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}
```

**Rules:**
- Every stage closes its output channel when done
- Every stage selects on `ctx.Done()` for cancellation
- Downstream reads with `range` — terminates when upstream closes

### Fan-Out / Fan-In

```go
// Fan-out: multiple goroutines reading from the same channel
workers := make([]<-chan Result, numWorkers)
for i := 0; i < numWorkers; i++ {
    workers[i] = worker(ctx, jobs)
}

// Fan-in: merge multiple channels into one
func merge(ctx context.Context, channels ...<-chan Result) <-chan Result {
    var wg sync.WaitGroup
    merged := make(chan Result)

    output := func(ch <-chan Result) {
        defer wg.Done()
        for val := range ch {
            select {
            case merged <- val:
            case <-ctx.Done():
                return
            }
        }
    }

    wg.Add(len(channels))
    for _, ch := range channels {
        go output(ch)
    }

    go func() {
        wg.Wait()
        close(merged)
    }()

    return merged
}
```

### Worker Pool — Bounded Concurrency

```go
func workerPool(ctx context.Context, jobs <-chan Job, results chan<- Result, numWorkers int) {
    var wg sync.WaitGroup
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for job := range jobs {
                select {
                case results <- process(job):
                case <-ctx.Done():
                    return
                }
            }
        }()
    }
    go func() {
        wg.Wait()
        close(results)
    }()
}
```

**When to use:** bounded resource access (DB connections, HTTP clients, CPU-bound work).

### Rate Limiting

```go
// Token bucket via time.Ticker
limiter := time.NewTicker(time.Second / ratePerSecond)
defer limiter.Stop()

for req := range requests {
    <-limiter.C // blocks until next tick
    go handle(req)
}

// Bursty: buffered channel as token bucket
bursty := make(chan struct{}, burstSize)
go func() {
    for {
        select {
        case bursty <- struct{}{}:
        case <-ctx.Done():
            return
        }
        time.Sleep(time.Second / ratePerSecond)
    }
}()
```

For production, prefer `golang.org/x/time/rate`:
```go
limiter := rate.NewLimiter(rate.Every(time.Second/10), burstSize)
if err := limiter.Wait(ctx); err != nil {
    return err
}
```

### Graceful Shutdown

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Trap OS signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    server := startServer(ctx)

    <-sigCh // block until signal
    cancel() // propagate cancellation

    // Give in-flight work time to drain
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    if err := server.Shutdown(shutdownCtx); err != nil {
        log.Fatalf("forced shutdown: %v", err)
    }
}
```

**Rules:**
- Signal triggers context cancellation
- All goroutines select on `ctx.Done()` and exit cleanly
- Use a timeout to bound graceful period — don't wait forever
- Drain channels / finish in-progress work before exiting

### Context Propagation Best Practices

| Rule | Why |
|------|-----|
| Pass `ctx` as first param to every function | Enables cancellation propagation |
| Never store `ctx` in a struct | Contexts are request-scoped, not object-scoped |
| Use `context.WithTimeout` at call sites | Caller controls deadline, not callee |
| Check `ctx.Err()` before expensive operations | Avoid wasted work after cancellation |
| Derive child contexts for sub-operations | Each can have tighter deadlines |
| Never pass `nil` context — use `context.TODO()` | Signals intentional placeholder |

```go
// Good: caller sets deadline
ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
defer cancel()
result, err := service.Fetch(ctx, id)

// Good: check before expensive work
func (s *Service) Process(ctx context.Context, data []Item) error {
    for _, item := range data {
        if ctx.Err() != nil {
            return ctx.Err()
        }
        // expensive operation
    }
    return nil
}
```

### Semaphore Pattern — Bounded Parallelism Without Worker Pool

```go
sem := make(chan struct{}, maxConcurrent)

for _, task := range tasks {
    sem <- struct{}{} // acquire
    go func(t Task) {
        defer func() { <-sem }() // release
        process(t)
    }(task)
}

// Wait for all to finish
for i := 0; i < maxConcurrent; i++ {
    sem <- struct{}{}
}
```

Or use `golang.org/x/sync/semaphore` for weighted semaphores.

### sync.Cond — Wait for Condition Changes

```go
type Buffer struct {
    mu    sync.Mutex
    cond  *sync.Cond
    data  []int
    maxSz int
}

func NewBuffer(size int) *Buffer {
    b := &Buffer{maxSz: size}
    b.cond = sync.NewCond(&b.mu)
    return b
}

func (b *Buffer) Put(val int) {
    b.mu.Lock()
    defer b.mu.Unlock()
    for len(b.data) == b.maxSz {
        b.cond.Wait() // releases lock, waits, reacquires
    }
    b.data = append(b.data, val)
    b.cond.Signal() // wake one waiter
}

func (b *Buffer) Get() int {
    b.mu.Lock()
    defer b.mu.Unlock()
    for len(b.data) == 0 {
        b.cond.Wait()
    }
    val := b.data[0]
    b.data = b.data[1:]
    b.cond.Signal()
    return val
}
```

**When to use:** producer-consumer with bounded buffer, waiting for state transitions.

---

## Common Concurrency Pitfalls

### Goroutine Leaks

**Cause:** goroutine blocks forever on channel send/receive with no cancellation path.

```go
// BAD: leaks if nobody reads from ch
func leak() <-chan int {
    ch := make(chan int)
    go func() {
        ch <- expensiveComputation() // blocks forever if consumer disappears
    }()
    return ch
}

// GOOD: select on context
func noLeak(ctx context.Context) <-chan int {
    ch := make(chan int, 1) // buffered so goroutine can exit even if consumer is gone
    go func() {
        select {
        case ch <- expensiveComputation():
        case <-ctx.Done():
        }
    }()
    return ch
}
```

**Detection:** monitor goroutine count with `runtime.NumGoroutine()` in tests.

### Deadlocks

**Common causes:**
1. **Lock ordering violation** — goroutine A locks X then Y, goroutine B locks Y then X
2. **Self-deadlock** — calling a method that acquires a lock from within a locked section
3. **Unbuffered channel with no receiver** — sender blocks forever

```go
// BAD: inconsistent lock ordering → deadlock
func (a *Account) Transfer(b *Account, amount int) {
    a.mu.Lock()
    b.mu.Lock() // if b.Transfer(a, ...) runs concurrently → deadlock
    // ...
}

// GOOD: always lock in consistent order (e.g., by ID)
func Transfer(a, b *Account, amount int) {
    first, second := a, b
    if a.id > b.id {
        first, second = b, a
    }
    first.mu.Lock()
    defer first.mu.Unlock()
    second.mu.Lock()
    defer second.mu.Unlock()
    // ...
}
```

### Race Conditions

**Cause:** unsynchronized read/write to shared state.

```go
// BAD: data race
var counter int
for i := 0; i < 1000; i++ {
    go func() { counter++ }()
}

// GOOD: atomic
var counter atomic.Int64
for i := 0; i < 1000; i++ {
    go func() { counter.Add(1) }()
}
```

### Channel Pitfalls

| Mistake | Fix |
|---------|-----|
| Sending on nil channel (blocks forever) | Always initialize channels |
| Closing a nil channel (panic) | Check before close |
| Closing an already-closed channel (panic) | Use `sync.Once` for close |
| Sending on closed channel (panic) | Sender owns the channel; only sender closes |
| Range over unclosed channel (blocks forever) | Ensure producer calls `close()` |

**Rule:** the sender closes the channel, never the receiver.

### Mutex Pitfalls

| Mistake | Fix |
|---------|-----|
| Forgetting to unlock (especially on error paths) | Always `defer mu.Unlock()` immediately after Lock |
| Copying a mutex (value receiver) | Use pointer receivers for types with mutex |
| Holding lock during I/O | Minimize critical section — lock, copy, unlock, then do I/O |
| Nested locks without consistent ordering | Document and enforce lock hierarchy |

---

## Concurrency Decision Matrix

| Scenario | Primitive |
|----------|-----------|
| Protect struct fields from concurrent access | `sync.Mutex` / `sync.RWMutex` |
| Simple counter/flag | `atomic.Int64` / `atomic.Bool` |
| One-time initialization | `sync.Once` |
| Wait for N goroutines to finish | `sync.WaitGroup` |
| Wait for N goroutines, fail-fast on error | `errgroup.Group` |
| Communicate data between goroutines | Channels |
| Bound concurrent access to resource | Semaphore (buffered chan or `semaphore.Weighted`) |
| Wait for arbitrary condition | `sync.Cond` |
| Reuse expensive temporary objects | `sync.Pool` |
| Periodic work with cancellation | `time.Ticker` + `select` on `ctx.Done()` |
| Request-scoped deadline propagation | `context.WithTimeout` / `context.WithDeadline` |
| Multiple concurrent operations, take fastest | `select` on multiple channels |

---

## Code Style

### Naming
- Interfaces: verb/capability (`Reader`, `Evictor`, `Allocator`)
- Structs: noun (`Pool`, `Cache`, `Limiter`)
- Constructors: `New` or `NewXxx`
- Errors: `ErrXxx` (sentinel) or custom types implementing `error`
- Files: one primary type per file, named after the type

### Project Structure (Layered Architecture)

Prefer this folder layout when the problem has multiple layers. For simpler problems, a flat structure is fine.

```
project/
├── go.mod
├── main.go              # Entry point — wires dependencies, starts the system
├── models/              # Pure data structs (no business logic)
├── services/            # Business logic (interface + unexported implementation)
├── handlers/            # Thin transport layer (HTTP/gRPC — decode, delegate, encode)
└── <entity>_test.go     # Tests alongside source
```

### Layer Dependency Direction

`main.go` → `handlers/` → `services/` (via interface) → `models/`

- Models know nothing about other layers
- Services depend only on models
- Handlers depend on service interfaces, never concrete types
- `main.go` wires everything — no business logic lives here

### File Naming
- Models: `<entity>.go`
- Services: `<entity>Service.go`
- Handlers: `<entity>Handler.go`
- Tests: `<entity>_test.go` in same package

### Error Handling
- Define sentinel errors for expected failure modes
- Wrap with context: `fmt.Errorf("operation: %w", err)`
- Custom error types when callers need to inspect details
- Never panic in library code
- Never ignore errors: handle or propagate

### Constructor Pattern (Dependency Injection)
```go
type Service struct {
    repo Repository  // interface, not concrete
}

func NewService(repo Repository) *Service {
    return &Service{repo: repo}
}
```

### Functional Options (for complex config)
```go
type Option func(*Config)

func WithMaxSize(n int) Option {
    return func(c *Config) { c.MaxSize = n }
}

func New(opts ...Option) *Thing {
    cfg := defaultConfig()
    for _, o := range opts {
        o(&cfg)
    }
    return &Thing{config: cfg}
}
```

---

## Testing Standards

- **Table-driven tests** as default pattern
- Test **behavior**, not implementation
- Use `t.Parallel()` for independent tests
- Use `t.Helper()` in test utilities
- Concurrency tests: spawn N goroutines, use `sync.WaitGroup`, run with `-race`
- Mock via interfaces — define the mock in `_test.go`
- Name: `TestXxx_condition_expectedResult`
- Aim for: happy path + error path + edge cases + concurrent access

---

## What NOT To Do

- No global mutable state (package-level vars that get mutated)
- No `init()` functions (prefer explicit initialization)
- No interface pollution (don't define interfaces you don't consume)
- No premature abstraction (need 3 concrete cases first)
- No `context.Background()` in library code — accept `ctx` from callers
- No hardcoded values — use config structs with sensible defaults
- No `if/else` chains for type variations — use polymorphism/Strategy
- No god structs that do everything
- No skipping concurrency in multi-user/multi-goroutine systems
