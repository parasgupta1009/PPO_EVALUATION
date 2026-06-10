# Skill: Explain Code Changes

## Description
Explain what was built in this KV store project — architecture, design decisions, data flow, and how components connect. Use when the user asks "what did we build", "explain the code", "how does X work", or "walk me through this".

## Project Overview

**What:** An in-memory key-value store with TTL expiration, exposed via HTTP REST API.
**Language:** Go | **Module:** `github.com/PPO_EVALUATION`

## Architecture (Layered)

```
main.go → handlers/ → services/ (via Store interface) → models/
```

- `models/` — Pure data structs, zero logic dependencies
- `services/` — Business logic behind an interface, thread-safe
- `handlers/` — Thin HTTP transport, decode → delegate → encode

## File Map

| File | Role |
|------|------|
| `main.go` | Wires dependencies, starts HTTP server, handles graceful shutdown |
| `models/entry.go` | `Entry{Value, ExpiresAt}` + `IsExpired()` method |
| `models/errors.go` | Sentinel errors: `ErrKeyNotFound`, `ErrKeyExpired` |
| `services/store.go` | `Store` interface + `kvStore` implementation with RWMutex |
| `services/store_test.go` | Unit tests: happy path, errors, concurrency, background cleanup |
| `handlers/store_handler.go` | HTTP handlers for POST/GET/DELETE with JSON responses |
| `handlers/store_handler_test.go` | E2E tests using `httptest.Server` |
| `test_flow.sh` | Bash script testing all 13 paths via curl |

## API Endpoints

| Method | Path | Success | Errors |
|--------|------|---------|--------|
| POST | `/keys` | 201 `{"message":"key set successfully","key":"..."}` | 400 (validation) |
| GET | `/keys/{key}` | 200 `{"key":"...","value":"..."}` | 404 (not found), 410 (expired) |
| DELETE | `/keys/{key}` | 200 `{"message":"deleted successfully","key":"..."}` | 404 (not found), 410 (expired) |

## Key Design Decisions

### 1. Store Interface (Interface Segregation)
```go
type Store interface {
    Set(key string, value string, ttl time.Duration)
    Get(key string) (string, error)
    Delete(key string) error
}
```
3 methods only. Handler depends on this interface, not the concrete `kvStore`.

### 2. Absolute Expiry Time
`Entry.ExpiresAt` stores `time.Time` (not duration). O(1) expiry check — just compare against `time.Now()`.

### 3. Hybrid Expiration Strategy
- **Lazy deletion:** `Get()` and `Delete()` check expiry on access, remove if expired
- **Active cleanup:** Background goroutine scans map on a ticker, purges stale keys
- Prevents memory leaks from write-only keys

### 4. Concurrency Model
- `sync.RWMutex` — multiple readers, exclusive writer
- `Get()` uses RLock for the read, upgrades to Lock only if expired (double-check pattern)
- `Set()` and `Delete()` use Lock
- Background cleanup uses Lock
- All tested with `-race` flag

### 5. Delete Returns Error
`Delete` checks existence and expiry before removing:
- Key exists & valid → delete, return `nil`
- Key not found → return `ErrKeyNotFound`
- Key expired → cleanup, return `ErrKeyExpired`

### 6. Graceful Shutdown
- Background cleanup goroutine bound to `context.Context`
- `main.go` traps SIGINT/SIGTERM, cancels context, calls `server.Shutdown()`
- No goroutine leaks

### 7. Custom removeKey Method
All deletions go through `s.removeKey(key)` — single point for future eviction hooks/logging.

## Data Flow Examples

### SET Flow
```
Client POST /keys {"key":"x","value":"v","ttl_seconds":60}
  → handler.Set() validates request
  → store.Set("x", "v", 60s)
  → acquires mu.Lock
  → data["x"] = Entry{Value:"v", ExpiresAt: now+60s}
  → releases lock
  → 201 {"message":"key set successfully"}
```

### GET Flow (expired key)
```
Client GET /keys/x
  → handler.Get() extracts key from path
  → store.Get("x")
  → acquires mu.RLock, finds entry, releases
  → entry.IsExpired() == true
  → acquires mu.Lock, double-checks, removeKey("x"), releases
  → returns "", ErrKeyExpired
  → handler writes 410 {"error":"key expired"}
```

## Testing Coverage

- **9 unit tests** in `services/` — happy path, error cases, concurrency (100 goroutines), background cleanup
- **10 E2E tests** in `handlers/` — full HTTP round-trips via `httptest.Server`
- **13 curl-based tests** in `test_flow.sh` — all paths including TTL expiry waits

## SOLID Principles Applied

| Principle | How |
|-----------|-----|
| **S**ingle Responsibility | models=data, services=logic, handlers=transport |
| **O**pen/Closed | New store types implement `Store` interface without modifying existing code |
| **L**iskov Substitution | Any `Store` impl swappable in handler |
| **I**nterface Segregation | 3-method interface, handlers only depend on what they use |
| **D**ependency Inversion | Handler depends on `Store` interface, not `*kvStore` |
