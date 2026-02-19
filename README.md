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
HTTP API </br>
↓</br>
Rate Limiter ← proactive backpressure begins here  </br>
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

## Day 3: Proactive Backpressure with Rate Limiting
### Setup

A simple token-bucket style rate limiter was introduced
before queue admission.

```go
limiter := ratelimit.New(3, time.Second)
```
- Maximum 3 requests per second
- Excess requests receive 429 Too Many Requests
- Queue admission remains non-blocking
The request flow became:

Client

→ HTTP

→ Rate Limiter

→ Non-blocking Queue Admission

→ Worker

### Observed Behavior
When sending 10 rapid requests:
```powershell
202
202
202
429
429
429
429
429
429
429
```
Key observation:
- The queue was not necessarily full
- Rejections occurred due to policy, not capacity
- Latency did not increase
- Handlers did not block
- The network layer remained stable

### Reactive vs Proactive Backpressure
| Type      | Triggered When   | Behavior                  | Risk Level |
| --------- | ---------------- | ------------------------- | ---------- |
| Reactive  | Queue full       | Rejects after saturation  | Medium     |
| Blocking  | Queue full       | Latency accumulation      | High       |
| Proactive | Policy threshold | Rejects before saturation | Low        |

## Stress Scenario Analysis
## Scenario 1: 100 Clients Send Requests Simultaneously
### Without Rate Limiting
- All requests reach the handler
- Queue fills quickly
- If blocking:
    - Connections remain open
    - Latency increases
    - Goroutines accumulate
    - Risk of connection exhaustion 
- If non-blocking:
    - Massive burst of 429 responses
    - Queue still briefly saturated
System behavior: Reactive and unstable under burst load.

### With Rate Limiting
- Only a bounded number of requests enter the system per second
- Queue pressure grows slowly or remains stable
- Worker pool remains within safe operating limits
- Rejections are predictable and controlled
System behavior: Stable under burst load.

## Scenario 2: Clients Retry Aggressively

### Without Rate Limiting

When clients retry immediately after rejection or timeout:

- Traffic multiplies (retry amplification)
- Queue saturates repeatedly
- Latency increases further
- Collapse becomes likely

This creates a **positive feedback loop**:

More retries → More pressure → More failures → More retries

---

### With Rate Limiting

- Retry attempts are also limited
- Feedback loop is dampened
- System load remains bounded
- Recovery becomes possible

Rate limiting acts as a **circuit breaker at the network boundary**.

---

## Scenario 3: Low Client Timeouts

Assume clients timeout after **500ms**.

### Blocking Admission (Day 2 Design)

- Clients timeout before receiving a response
- They retry
- Original request may still be processing
- Duplicate load increases
- System destabilizes rapidly

---

### With Rate Limiting

- Rejections happen immediately
- Clients receive explicit overload signals
- Properly implemented clients back off
- Duplicate processing risk decreases

---

## Key Insights from Day 3

- Rate limiting transforms overload from a failure into a policy decision.
- Proactive rejection is healthier than reactive rejection.
- Latency-based backpressure is dangerous and invisible.
- Retry amplification is one of the most destructive failure patterns.
- Stability depends more on admission control than on raw throughput.

---

## Day 4: Scaling, Capacity, and the Role of Rate Limiting

After introducing rate limiting, we explored how scaling interacts with system capacity.

### Key Question

If the business requires 50 requests per second (RPS),
should we just increase the rate limit to 50?

The answer depends on **processing capacity**, not policy.

---

## Capacity vs Rate Limit

True system capacity is defined by processing power:

Capacity = worker_count × worker_throughput


Example:

- 1 worker
- Each task takes 500ms
→ ~2 RPS per worker

To support 50 RPS:

50 / 2 = 25 workers (theoretical minimum)


Simply increasing the rate limit to 50 without scaling workers
would overload the system.

> Rate limit does not increase capacity.  
> It only controls admission.

---

## Why Rate Limit ≠ Capacity

Rate limiting is a **stability mechanism**.

It defines how much traffic is allowed into the system,
but it does not determine how fast the system can process tasks.

If:

- Capacity = 2 RPS
- Rate limit = 50 RPS

Then:

- 50 requests enter
- Only 2 are processed per second
- Queue fills
- Latency increases
- System destabilizes

Increasing the rate limit without scaling
only accelerates failure.

---

## Why Rate Limit Should Be Slightly Below Capacity

Even if theoretical capacity is 50 RPS,
real systems fluctuate due to:

- CPU spikes
- GC pauses
- Lock contention
- Network jitter

Therefore, rate limit should usually be:

~70–90% of sustainable capacity


This creates safety headroom and improves stability.

---

## Large Queue ≠ Stability

We also considered increasing queue size (e.g., 1000).

This does not improve stability.

Instead, it:

- Hides overload
- Increases latency
- Increases memory pressure
- Delays failure instead of preventing it

A large queue absorbs bursts,
but it does not solve sustained overload.

> A large queue hides overload.  
> A rate limiter prevents overload.

---

## Scaling vs Rate Limiting

| Problem | Solution |
|----------|----------|
| Processing too slow | Scaling |
| Burst traffic | Rate limiting |
| Retry amplification | Explicit 429 |
| Latency accumulation | Non-blocking admission |

---

## Core Insights So Far

1. Scaling increases throughput.
2. Rate limiting preserves stability.
3. Scaling does not replace admission control.
4. Rate limit must reflect sustainable capacity.
5. Stability is more important than raw throughput.

---

## System Evolution Summary

- Day 1: Reactive backpressure (queue-based rejection)
- Day 2: Blocking admission (latency-based overload)
- Day 3: Proactive rate limiting
- Day 4: Capacity modeling and scaling analysis

The system now demonstrates:

- Explicit overload signaling
- Bounded resource usage
- Controlled admission
- Capacity-aware scaling decisions

---

## Day 5: True Graceful Shutdown & 3-State Idempotency

Today the system was upgraded to production-grade behavior.

### 1) Proper Worker Drain

Shutdown sequence was redesigned:

1. Stop HTTP admission
2. Close internal queue
3. Workers drain remaining tasks
4. WaitGroup ensures all workers exit
5. Process exits cleanly

This guarantees:

- No task is silently lost
- In-flight processing completes
- Bounded and predictable shutdown behavior

---

### 2) 3-State Idempotency Model

Idempotency was upgraded from a simple boolean flag to a state machine:

NotSeen → Processing → Processed


Behavior:

- First request → enqueue → state = Processing
- Duplicate during processing → 409 Conflict ("still processing")
- After worker completion → state = Processed
- Duplicate after completion → 200 OK ("already processed")

This prevents:

- Double enqueue
- Duplicate execution
- Race conditions during retries

---

### 3) Correct Layering

Idempotency responsibilities were moved to proper layers:

- Handler: admission + duplicate check
- Worker: marks completion
- Store: thread-safe state machine

This separates transport-level control from business-level completion.

---

## Final System Capabilities

The system now demonstrates:

- Explicit backpressure via HTTP 429
- Rate-limited admission control
- Bounded internal queue
- Capacity-aware scaling behavior
- Graceful shutdown with worker drain
- 3-state idempotent task processing

---

## What This Lab Proved

1. Scaling does not guarantee stability.
2. Rate limiting protects capacity.
3. Large queues hide overload but amplify latency.
4. Graceful shutdown requires layered coordination.
5. Idempotency must be modeled as a state machine.
6. Data integrity ultimately belongs to the persistence layer.

---

## Mental Model (Final)

Client  

↓  

HTTP Admission (Rate Limit + Idempotency Check)  

↓  

Bounded Queue  

↓  

Worker Pool  

↓  

Completion Marking  

Pressure is absorbed at the edges.  
Integrity is enforced at completion.








