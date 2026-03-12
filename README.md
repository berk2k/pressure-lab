# pressure-lab

An experimental Go project to study how a **network-facing backend system** behaves under load, focusing on **backpressure propagation, admission control, latency, and recovery**.

This is not a production system.
This is a learning lab built step by step to observe *system behavior* — not to optimize throughput.

---

## Core Question

> Where does pressure accumulate, and how does it affect upstream clients?

---

## Architecture

```
Client
  ↓
HTTP API
  ↓
Rate Limiter       ← proactive backpressure begins here
  ↓
Admission Control  ← idempotency check + duplicate detection
  ↓
Bounded Queue      ← buffered channel
  ↓
Worker Pool        ← completion marking
```

---

## Features

### Admission Control
- Non-blocking queue admission (fast reject on full)
- Token-bucket rate limiting (proactive rejection)
- HTTP 429 on overload — explicit, observable backpressure

### Idempotency
- 3-state task model: `NotSeen → Processing → Processed`
- Duplicate during processing → 409 Conflict
- Duplicate after completion → 200 OK
- Thread-safe state machine

### Worker Pool
- Bounded concurrency
- Capacity-aware scaling analysis
- Processes tasks from internal queue

### Graceful Shutdown
- HTTP admission stops first
- Queue is closed
- Workers drain remaining tasks
- WaitGroup ensures all workers exit before process terminates

### Observability
- Request acceptance / rejection rates visible via HTTP status codes
- Queue depth observable at runtime

---

## How to Run

### Prerequisites

- Go 1.21+

### Clone and run

```bash
git clone https://github.com/berk2k/pressure-lab.git
cd pressure-lab
go run cmd/main.go
```

### Send a request

```bash
curl -X POST http://localhost:8080/submit -H "Idempotency-Key: task-1"
```

### Burst test

```bash
for i in $(seq 1 10); do curl -s -w "%{http_code}\n" -X POST http://localhost:8080/submit -H "Idempotency-Key: task-$i"; done
```

### Duplicate test

```bash
# Send the same key twice
curl -X POST http://localhost:8080/submit -H "Idempotency-Key: task-1"
# While processing  → 409 Conflict
# After completion  → 200 OK
```

### Expected output under rate limit

```
202
202
202
429
429
429
429
```

---

## Observed Behaviors

| Scenario | Behavior |
|---|---|
| Queue full, non-blocking admission | Immediate 429 |
| Queue full, blocking admission | Silent latency buildup |
| Rate limit active | 429 before queue even fills |
| Duplicate request (processing) | 409 Conflict |
| Duplicate request (completed) | 200 OK |
| Graceful shutdown | All in-flight tasks complete before exit |

---

## Capacity Model

System throughput is determined by processing power, not rate limit:

```
Capacity = worker_count × worker_throughput
```

Example: 1 worker × 2 RPS = 2 RPS sustainable capacity.

> Rate limit controls admission. Scaling controls capacity. They are not interchangeable.

---

## What This Lab Studies

- Explicit vs implicit (latency-based) backpressure
- Reactive vs proactive admission control
- Retry amplification and positive feedback loops
- Capacity modeling vs rate limiting
- Graceful shutdown sequencing
- Idempotency as a state machine

---

## Current Limitations

- In-memory only (no persistence)
- No distributed coordination
- Idempotency state is lost on restart
- No metrics endpoint (stdout only)

---

For detailed design decisions and trade-offs, see:

👉 [DESIGN.md](DESIGN.md)
