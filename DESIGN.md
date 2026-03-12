# pressure-lab – Design & Trade-offs

This document explains the internal design decisions, trade-offs, and alternative approaches explored while building this lab.

The goal is not to build a production API gateway, but to deeply understand how backpressure propagates through a network-facing system and where pressure accumulates under load.

---

# 1: Admission Control

## Why Non-Blocking Queue Admission?

The HTTP handler uses a non-blocking channel send:

```go
select {
case queue <- task:
    // accepted → 202
default:
    // queue full → 429
}
```

### Why?
- The HTTP handler must never block waiting for queue space
- Blocking a handler holds an open connection, a goroutine, and a file descriptor
- Under load, blocked handlers accumulate silently — the system appears healthy while becoming unstable

### What blocking looks like (Day 2 experiment):

```go
queue <- task // blocking send
```

- All requests returned 202
- No visible errors
- But: latency increased, connections accumulated, goroutines stacked

The system appeared healthy while actively degrading. This is the most dangerous failure mode — invisible overload.

### Trade-off of non-blocking (fast reject):
- Clients receive 429 instead of waiting
- Requires clients to handle rejection gracefully (retry with backoff)
- Without well-behaved clients, retry amplification can occur

### Alternative considered:
- Timeout-based admission (block for N ms, then reject)
- Provides a small buffer for transient bursts
- Rejected: adds latency uncertainty; fast reject is more predictable

---

# 2: Backpressure Forms

## Explicit vs Implicit Backpressure

Two backpressure forms were intentionally compared in this lab:

| Form | Mechanism | Client Experience | Stability |
|---|---|---|---|
| Explicit | HTTP 429 | Immediate signal | Predictable |
| Implicit | Latency increase | Silent waiting | Fragile |
| Proactive | Rate limit 429 | Rejection before saturation | Most stable |

### Why explicit backpressure is better

Implicit (latency-based) backpressure has a core problem:

> The system looks healthy from the outside while degrading on the inside.

Metrics show 200s. Clients aren't seeing errors. But:
- Goroutines accumulate
- Connections stay open
- Memory grows
- Eventually the system collapses without warning

Explicit 429 backpressure:
- Communicates overload clearly
- Allows well-behaved clients to back off
- Keeps the network layer healthy (connections close immediately)
- Makes failure visible and observable

### Trade-off:
- Requires clients to implement retry logic with backoff
- Poor clients will retry immediately → retry amplification
- Mitigated by rate limiting at the boundary

---

# 3: Rate Limiting

## Why Proactive Rate Limiting?

A token-bucket rate limiter sits before queue admission:

```
Client → Rate Limiter → Queue Admission → Worker
```

Reactive backpressure (queue-full 429) triggers only after saturation.
Proactive rate limiting rejects before the queue fills.

### Why?
- Under burst traffic, a queue fills in milliseconds
- Reactive rejection means the system has already been overloaded briefly
- Rate limiting keeps load bounded at the edge — workers never see burst pressure

### Observed behavior with 10 simultaneous requests and rate limit of 3 RPS:
```
202
202
202
429
429
429
429
```

- Queue was not necessarily full
- Rejections were policy-based, not capacity-based
- No latency increase

### Trade-off:
- Rate limit must be calibrated to actual processing capacity
- Too low: unnecessary rejections, underutilized workers
- Too high: doesn't protect against saturation

### Why rate limit should be below theoretical capacity:

Real systems fluctuate due to GC pauses, CPU spikes, lock contention, network jitter.

Rate limit should target ~70–90% of sustainable capacity to maintain a safety margin.

### Alternative considered:
- Leaky bucket (smoothed output rate)
- More predictable per-request pacing
- Adds complexity; token bucket sufficient for this scope

---

# 4: Queue Design

## Why a Bounded Buffered Channel?

The internal queue is a `chan Task` with fixed capacity.

### Why bounded?
- Unbounded queue hides overload — it absorbs all traffic while latency grows
- A large queue does not prevent failure; it delays it
- Bounded queue makes overload explicit and immediately observable

### The large queue trap:

A queue of 1000 with 2 RPS processing capacity:
- Absorbs 1000 requests
- Takes 500 seconds to drain
- Clients wait up to 500s for a response
- Every queued request holds a connection open
- Memory grows

> A large queue hides overload. A rate limiter prevents overload.

### Trade-off:
- Small queue = more 429s under burst
- Large queue = burst absorption at the cost of latency and memory

Queue size should reflect the burst tolerance acceptable to the system, not be used as a stability mechanism.

### Alternative considered:
- Priority queue (high-priority tasks skip ahead)
- Relevant for mixed workload systems
- Out of scope for this lab

---

# 5: Capacity Modeling

## Rate Limit ≠ Capacity

A key insight explored in Day 4:

```
Capacity = worker_count × worker_throughput
```

Increasing the rate limit without scaling workers does not increase throughput — it only accelerates queue saturation.

Example:
- 1 worker × 500ms/task = 2 RPS capacity
- Rate limit set to 50 RPS
- 50 requests enter per second, 2 are processed
- Queue fills in seconds
- System destabilizes

### Why this matters:

Rate limiting is a **stability mechanism**.
Scaling is a **capacity mechanism**.
They serve different purposes and cannot substitute for each other.

### Trade-off of over-limiting:
- Setting rate limit well below capacity leaves throughput on the table
- Wasted worker cycles

### Trade-off of under-limiting:
- Queue saturates
- Latency spikes
- Collapse risk under sustained load

---

# 6: Retry Amplification

## Why Retry Amplification is Dangerous

When clients retry rejected requests immediately without backoff:

```
More rejections → More retries → More load → More rejections
```

This is a positive feedback loop. Under burst conditions it can:
- Multiply effective traffic 3–10x
- Cause the system to fail at load levels it would otherwise handle

### Rate limiting as a circuit breaker:

With rate limiting active, retry attempts are also throttled.
- The feedback loop is dampened
- Load remains bounded
- Recovery becomes possible

Without rate limiting, retry amplification can prevent recovery entirely.

### Trade-off:
- Client-side backoff is required for this to work correctly
- Servers cannot force clients to back off — only signal overload clearly
- Explicit 429 with `Retry-After` header is the correct signal

---

# 7: Idempotency Design

## Why a 3-State Model?

A boolean "seen/not seen" idempotency check is insufficient for concurrent systems.

The lab uses a 3-state model:

```
NotSeen → Processing → Processed
```

| State | Behavior on duplicate |
|---|---|
| NotSeen | Enqueue normally |
| Processing | 409 Conflict ("still processing") |
| Processed | 200 OK ("already done") |

### Why?
- Between enqueue and completion, a duplicate request arrives
- Boolean check: second request would be blocked, but first hasn't completed
- 3-state model communicates the correct system state to the client
- Prevents double execution and race conditions during retries

### Trade-off:
- State is in-memory — lost on restart
- For true durability, state must be persisted (database, Redis)
- In-memory is acceptable for this lab scope

### Alternative considered:
- Distributed idempotency store (Redis)
- Enables restart-safe deduplication
- Adds infrastructure dependency; future improvement

---

# 8: Graceful Shutdown

## Why Layered Shutdown Sequencing?

Shutdown follows a strict ordering:

1. Stop HTTP admission (no new requests accepted)
2. Close internal queue channel (signals workers)
3. Workers drain remaining tasks from queue
4. WaitGroup blocks until all workers complete
5. Process exits

### Why this order?

- Closing the queue before stopping admission risks panics (send on closed channel)
- Stopping workers before draining loses in-flight tasks
- WaitGroup ensures no task is silently discarded

### Trade-off:
- Shutdown time is bounded by the longest in-flight task
- No forced timeout in current implementation

### Alternative considered:
- Timeout-based forced drain (similar to broker's drain window)
- After N seconds, remaining queued tasks are dropped
- Acceptable trade-off in production; omitted here to preserve zero-loss semantics

---

# 9: Concurrency Model

## Why Mutex for Idempotency Store?

The idempotency state map is protected by `sync.Mutex`.

### Why?
- Multiple goroutines (HTTP handlers) read and write concurrently
- Map access without synchronization is a data race in Go
- Mutex is the simplest correct solution

### Trade-off:
- Lock contention grows with concurrent request volume
- At high concurrency, a sharded map or `sync.Map` would reduce contention

### Alternative considered:
- `sync.RWMutex` — allows concurrent reads, serializes writes
- Worth adopting if read-heavy workload (e.g., many duplicate checks)

---

# 10: Observability

## Why HTTP Status Codes as the Primary Signal?

Backpressure state is communicated entirely through HTTP status codes:
- `202` — accepted
- `200` — already processed (idempotent)
- `409` — currently processing (idempotent conflict)
- `429` — rate limited or queue full

### Why?
- Clients can observe system state without any additional tooling
- Status codes are a standard, machine-readable signal
- Enables client-side backoff logic based on response codes

### Trade-off:
- No metrics endpoint (current implementation)
- Queue depth, rejection rate, worker utilization are not externally visible
- Blind spots in production monitoring

### Future improvement:
- Prometheus `/metrics` endpoint
- Gauges: queue depth, worker count, in-flight tasks
- Counters: accepted, rejected, processed, duplicates blocked

---

# 11: Known Limitations

- In-memory only — idempotency state lost on restart
- No distributed coordination (single process)
- No metrics endpoint
- No configurable parameters via environment variables
- No timeout-based forced drain on shutdown

These are intentional omissions to keep focus on backpressure and admission control mechanics.

---

# 12: Future Improvements

- Persistent idempotency store (Redis or database)
- Prometheus metrics endpoint
- Environment-variable-based configuration
- Timeout-based forced drain on shutdown
- `Retry-After` header on 429 responses
- Distributed rate limiting (multi-instance coordination)
- Priority queue for mixed workloads

---

# Systems Learning Outcomes

This lab demonstrates understanding of:

- Explicit vs implicit backpressure
- Reactive vs proactive admission control
- The queue size vs stability misconception
- Capacity modeling (rate limit ≠ capacity)
- Retry amplification and positive feedback loops
- 3-state idempotency for concurrent systems
- Layered graceful shutdown sequencing
- Concurrency safety in Go

---

# Closing Note

This lab is intentionally minimal.

Its purpose is to make visible the mechanics behind:

- API gateway admission control
- Kubernetes ingress rate limiting
- Queue-based worker systems under sustained load
- Idempotency patterns in distributed systems

By building these primitives manually, system behavior under pressure becomes explicit instead of hidden behind framework abstractions.
