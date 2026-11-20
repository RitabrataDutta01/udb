# Distributed Rate Limiter (Go + Redis)

A Token Bucket rate limiter implemented in Go, comparing a single-node **in-memory** limiter against a **distributed Redis-backed** limiter exposed via HTTP middleware.

## Project Structure

- `memory.go` — Thread-safe in-memory Token Bucket (`sync.Mutex`) for single-server use and benchmarking.
- `redis.go` — Distributed Token Bucket backed by Redis Hashes and an atomic Lua script.
- `middleware.go` — `net/http` rate-limiting middleware returning `429 Too Many Requests` with a `Retry-After` header.
- `main.go` — Runs a 50-goroutine concurrency comparison test, then starts the HTTP server on `:8080`.

## Key Design Decisions

- **Atomicity via Lua:** Read, refill, check, decrement, and write execute in a single Redis Lua script (`EVALSHA`), preventing read-modify-write race conditions across concurrent requests and multiple backend instances.
- **No Clock Skew:** Uses Redis's `TIME` command inside the Lua script as the single source of truth instead of application server clocks.
- **Automatic Cleanup:** Sets a `PEXPIRE` TTL equal to the full bucket refill duration (`maxTokens / refillRate`) so idle keys don't leak memory.
- **Fail-Open Middleware:** If Redis is unreachable, errors are logged and traffic is allowed through so a cache outage does not bring down the API.

## Prerequisites

- Go 1.21+
- Redis running on `localhost:6379`

## Running

Start the benchmark and HTTP server:

```bash
go run .
```

### Sample Concurrency Output (50 concurrent requests, bucket size 5)

```text
[In-Memory] Allowed: 5 | Denied: 45 | Errors: 0 | Time: 59.563µs
[Redis Lua] Allowed: 5 | Denied: 45 | Errors: 0 | Time: 3.848634ms | Retry-After: 20s
```

### Testing the HTTP Endpoint

Send 6 requests to `/api/data`:

```bash
# Bash / Zsh
for i in {1..6}; do curl -i http://localhost:8080/api/data; done
```

```fish
# Fish
for i in (seq 1 6); curl -i http://localhost:8080/api/data; end
```

Requests 1–5 return `200 OK`; request 6 returns `429 Too Many Requests` with `Retry-After: 20`.