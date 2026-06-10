# Plan: In-Memory Key-Value Store with TTL & HTTP API

## Context

Build a thread-safe in-memory key-value store in Go supporting SET (with TTL), GET, and DELETE operations. Expose via REST API. All operations return meaningful responses — expired keys return `410 Gone`, missing keys return `404 Not Found`. Apply SOLID principles, layered architecture, and safe concurrency patterns.

---

## Requirements

### Functional
- **SET(key, value, ttl)** — Store a key-value pair with a time-to-live duration
- **GET(key)** — Retrieve value; return error if key doesn't exist or is expired
- **DELETE(key)** — Remove a key; return error if key doesn't exist or is expired

### Non-Functional
- Thread-safe (concurrent access from multiple goroutines)
- SOLID principles enforced throughout
- Layered folder structure (models → services → handlers)
- Graceful shutdown with context cancellation
- Background cleanup of expired keys to prevent memory leaks

---

## Folder Structure

```
PPO_EVALUATION/
├── go.mod                          # module github.com/PPO_EVALUATION
├── main.go                         # Wiring, DI, HTTP server, graceful shutdown
├── models/
│   ├── entry.go                    # Entry struct (Value, ExpiresAt) + IsExpired()
│   └── errors.go                   # Sentinel errors: ErrKeyNotFound, ErrKeyExpired
├── services/
│   ├── store.go                    # Store interface + kvStore implementation
│   └── store_test.go              # Unit tests + concurrency tests
├── handlers/
│   ├── store_handler.go           # HTTP handlers (POST/GET/DELETE)
│   └── store_handler_test.go      # E2E tests with httptest.Server
└── test_flow.sh                    # Bash script for manual curl-based testing
```

---

## Implementation

### 1. `models/errors.go` — Sentinel Errors

```go
var (
    ErrKeyNotFound = errors.New("key not found")
    ErrKeyExpired  = errors.New("key expired")
)
```

Two distinct sentinels so callers can distinguish "never existed" from "existed but expired" using `errors.Is()`.

### 2. `models/entry.go` — Entry Value Object

```go
type Entry struct {
    Value     string
    ExpiresAt time.Time
}

func (e Entry) IsExpired() bool {
    return time.Now().After(e.ExpiresAt)
}
```

- Store absolute `time.Time` (not duration) for O(1) expiry check
- Pure value object — no mutex, no external dependencies

### 3. `services/store.go` — Core Implementation

**Interface (3 methods, per Interface Segregation):**
```go
type Store interface {
    Set(key string, value string, ttl time.Duration)
    Get(key string) (string, error)
    Delete(key string) error
}
```

**Struct:**
```go
type kvStore struct {
    mu   sync.RWMutex
    data map[string]models.Entry
}
```

**Constructor:**
```go
func NewKVStore(ctx context.Context, cleanupInterval time.Duration) *kvStore
```
- Initializes map, starts background cleanup goroutine bound to ctx

**Set:**
- Acquires write lock
- Stores `Entry{Value, ExpiresAt: time.Now().Add(ttl)}`

**Get:**
- Acquires read lock, reads entry
- If not found → `ErrKeyNotFound`
- If expired → upgrades to write lock, double-check pattern, removes key → `ErrKeyExpired`
- Otherwise → returns value

**Delete:**
- Acquires write lock
- If not found → `ErrKeyNotFound`
- If expired → removes key → `ErrKeyExpired`
- Otherwise → removes key → `nil` (success)

**removeKey (private):**
- Centralized deletion method — single point for all map removals
- Caller must hold write lock

**Background Cleanup:**
- `time.Ticker` goroutine scans all keys, deletes expired entries
- Stops on `ctx.Done()` — no goroutine leaks

### 4. `handlers/store_handler.go` — HTTP Transport Layer

**Routes:**
```
POST   /keys        → Set
GET    /keys/{key}  → Get
DELETE /keys/{key}  → Delete
```

**Response Design:**

| Operation | Scenario | Status | Body |
|-----------|----------|--------|------|
| SET | Success | 201 | `{"message":"key set successfully","key":"..."}` |
| SET | Missing key | 400 | `{"error":"key is required"}` |
| SET | Invalid TTL | 400 | `{"error":"ttl_seconds must be positive"}` |
| SET | Bad JSON | 400 | `{"error":"invalid request body"}` |
| GET | Found | 200 | `{"key":"...","value":"..."}` |
| GET | Not found | 404 | `{"error":"key not found"}` |
| GET | Expired | 410 | `{"error":"key expired"}` |
| DELETE | Deleted | 200 | `{"message":"deleted successfully","key":"..."}` |
| DELETE | Not found | 404 | `{"error":"key not found"}` |
| DELETE | Expired | 410 | `{"error":"key expired"}` |

### 5. `main.go` — Entry Point

- Creates root context with `context.WithCancel`
- Traps SIGINT/SIGTERM for graceful shutdown
- Wires `NewKVStore(ctx, cleanupInterval)` → `NewStoreHandler(store)` → `http.ServeMux`
- Starts HTTP server on `:8080`
- On signal: cancels context (stops cleanup goroutine), calls `server.Shutdown()` with timeout
- Zero business logic — only dependency wiring

### 6. `services/store_test.go` — Unit Tests

| Test | Validates |
|------|-----------|
| `TestSet_Get_HappyPath` | Set then Get returns correct value |
| `TestGet_ErrorCases` | Table-driven: not found → ErrKeyNotFound, expired → ErrKeyExpired |
| `TestDelete_RemovesKey` | Delete returns nil, subsequent Get returns ErrKeyNotFound |
| `TestDelete_NonexistentKey` | Delete missing key returns ErrKeyNotFound |
| `TestDelete_ExpiredKey` | Delete expired key returns ErrKeyExpired |
| `TestSet_OverwriteExisting` | Second Set overwrites, Get returns latest |
| `TestConcurrent_SetGet` | 100 goroutines Set + Get, no race |
| `TestConcurrent_SetDelete` | 100 goroutines Set + Delete, no race |
| `TestBackgroundCleanup_RemovesExpired` | Expired key removed without explicit access |

### 7. `handlers/store_handler_test.go` — E2E Tests

| Test | Validates |
|------|-----------|
| `TestE2E_SetAndGet_HappyPath` | POST 201 + GET 200 with correct JSON |
| `TestE2E_Delete_HappyPath` | DELETE 200 + subsequent GET 404 |
| `TestE2E_Delete_KeyNotFound` | DELETE missing → 404 |
| `TestE2E_Delete_KeyExpired` | DELETE expired → 410 |
| `TestE2E_Get_KeyNotFound` | GET missing → 404 |
| `TestE2E_Get_KeyExpired` | GET expired → 410 |
| `TestE2E_Set_InvalidBody` | Bad JSON → 400 |
| `TestE2E_Set_MissingKey` | Empty key → 400 |
| `TestE2E_Set_InvalidTTL` | Zero TTL → 400 |
| `TestE2E_Set_OverwriteKey` | Overwrite returns latest value |

### 8. `test_flow.sh` — Manual Curl Test Script

Bash script testing all 13 paths with colored pass/fail output:
- 5 SET tests (1 happy + 4 validation errors)
- 4 GET tests (2 happy + not found + expired)
- 4 DELETE tests (1 happy + not found + expired + GET-after-delete)

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| `sync.RWMutex` | Reads dominate in KV stores; allows concurrent Gets |
| Absolute `time.Time` for expiry | O(1) comparison vs storing duration + creation time |
| Two sentinel errors | Callers distinguish "never existed" from "expired" |
| Delete returns error | Enables meaningful API responses (200/404/410) |
| Hybrid expiration (lazy + active) | Lazy = correct semantics on access; Active = prevents memory leaks |
| Double-check in Get after lock upgrade | Prevents TOCTOU race between RLock→Lock |
| Context-bound cleanup goroutine | Graceful shutdown, no goroutine leaks |
| Centralized `removeKey()` | Single deletion point for future hooks/metrics |
| Interface in services/ | Single consumer, keeps project simple while enabling mockability |
| Unexported `kvStore` struct | Forces construction through `NewKVStore`, ensures proper init |

## SOLID Principles Applied

| Principle | Implementation |
|-----------|---------------|
| **Single Responsibility** | models=data, services=logic, handlers=transport |
| **Open/Closed** | New store implementations via `Store` interface, no edits needed |
| **Liskov Substitution** | Any `Store` impl swappable without breaking handlers |
| **Interface Segregation** | 3-method interface, minimal surface area |
| **Dependency Inversion** | Handler depends on `Store` interface, not `*kvStore` |

---

## Concurrency Model

- `Set()` → `mu.Lock()` → write map → unlock
- `Get()` → `mu.RLock()` → read → unlock → if expired: `mu.Lock()` → double-check → remove → unlock
- `Delete()` → `mu.Lock()` → check exists → check expired → remove → unlock
- `removeExpired()` → `mu.Lock()` → scan all keys → remove stale → unlock
- Background goroutine → `select` on ticker + `ctx.Done()`

All operations tested with 100 concurrent goroutines under `-race` flag.

---

## Verification

```bash
# Lint and format
go vet ./...
gofmt -l .

# Run all tests with race detector
go test -race -v ./...

# Manual testing
go run main.go &
./test_flow.sh

# Expected: 19 unit/E2E tests pass + 13 curl tests pass
```

## Curl Reference

```bash
# SET
curl -X POST http://localhost:8080/keys \
  -H "Content-Type: application/json" \
  -d '{"key":"user:1","value":"Alice","ttl_seconds":60}'

# GET
curl http://localhost:8080/keys/user:1

# DELETE
curl -X DELETE http://localhost:8080/keys/user:1
```
