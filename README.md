# pressure-lab

An experimental Go project to study how a **network-facing backend system**
behaves under load, focusing on **backpressure propagation, admission control,
latency, and recovery**.

This is **not** a production system.
This repository is a **learning lab** built step by step to observe *system behavior*,
not to optimize throughput.

---

## Mental Model
Client </br>
↓</br>
HTTP API ← backpressure becomes visible here</br>
↓</br>
Admission Control</br>
↓</br>
Internal Queue (buffered channel)</br>
↓</br>
Worker Pool

Core question:

> Where does pressure accumulate, and how does it affect upstream clients?

---

## Day 1: Fast Reject / Explicit Backpressure

### Setup

- HTTP server in Go
- Bounded in-memory queue (buffered channel)
- Single slow worker (`500ms` per task)
- Non-blocking admission control

Key idea:

> **The HTTP handler must never block when the queue is full.**

### Admission Logic

```go
select {
case queue <- task:
    // accepted
default:
    // reject
}
```

### Observed Behavior
- Initial requests returned 202 Accepted
- Once capacity was reached, responses switched to 429 Too Many Requests
- No request waited
- No latency buildup
- No goroutine or connection accumulation

When queue size was reduced to 1:
- Exactly 2 requests were accepted:
  - 1 active worker
  - 1 queued task
- All subsequent requests were rejected immediately

### Key Learnings
- Backpressure is a response, not a delay
- Queue capacity ≠ total system capacity  (queue capacity + active workers)
- Fast rejection keeps the network layer healthy
- The system clearly communicates overload to client

## Day 2: Blocking Admission / Latency-Based Backpressure (Intentional Mistake)
### Change Introduced
The non-blocking admission was removed.
```go
queue <- task // blocking enqueue
```
- No 429 responses
- HTTP handlers block when the queue is full
- Clients wait instead of being rejected

### Observed Behavior
- All requests eventually returned 202 Accepted
- Requests experienced increasing latency
- Pressure accumulated silently as:
  - open connections
  - blocked handlers
  - delayed responses

From the client perspective:
- The system appeared "healthy"
- But responses became progressively slower

### Key Learnings

- Latency is a dangerous form of backpressure
- Blocking the handler hides overload instead of signaling it
- Open connections and waiting clients push the system toward collapse
- A system can look healthy while becoming unstable

In the blocking design, the system appeared healthy (202 responses),
but pressure silently accumulated as latency and open connections.

## Day 1 vs Day 2
| Aspect             | Day 1 (Fast Reject)  | Day 2 (Blocking)   |
| ------------------ | -------------------- | ------------------ |
| Backpressure form  | Explicit (429)       | Implicit (latency) |
| Client experience  | Immediate feedback   | Silent waiting     |
| Network health     | Protected            | At risk            |
| Failure visibility | Clear and observable | Hidden             |
| System stability   | Predictable          | Fragile            |

### Current Status
- Phase 0 complete
- Baseline and failure mode observed
- No rate limiting yet
- No graceful shutdown yet
