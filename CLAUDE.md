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
