# task-queue

`task-queue` is a small Go-based background job queue built around Redis. It currently supports:

- immediate tasks
- one-off scheduled tasks
- cron-style recurring tasks
- worker execution with retries and acknowledgements
- a scheduler loop for promoting due tasks
- a reaper loop for timed-out task inspection

This repository is still evolving and should be treated as a learning project rather than a production-ready queue.

## Architecture

The latest architecture diagram lives in [arch.svg](./arch.svg).

![task-queue architecture](./arch.svg)

The current runtime is organized like this:

1. A client submits a task request to `cmd/producer`.
2. The producer enriches the request with task metadata and writes it through the Redis-backed broker.
3. Redis stores each task as a hash and tracks task IDs across queue and sorted-set structures.
4. `cmd/worker` starts a worker pool and an embedded scheduler loop.
5. Workers block on the pending queue, move tasks into processing, execute registered handlers, then `Ack` or `Nack`.
6. The scheduler polls scheduled tasks and requeues due one-off and cron tasks into the pending queue.
7. `cmd/reaper` polls for expired task timeouts. Recovery and cleanup are still incomplete.

Task handlers are regular Go functions today. WASM-based task execution is planned, but it is not part of the current runtime yet.

## Components

### Commands

- `cmd/producer`
  Accepts CLI flags, builds a task request, and either enqueues or schedules the task.

- `cmd/worker`
  Starts the worker pool, registers task handlers, and runs the scheduler loop in the same process.

- `cmd/reaper`
  Starts the timeout reaper loop.

### Packages

- `pkg/task`
  Core task model, stages, kinds, and Redis serialization helpers.

- `pkg/producer`
  Creates tasks from request payloads and sends them to the broker.

- `pkg/broker`
  Broker interface plus the Redis implementation for enqueue, dequeue, ack/nack, scheduling, and timeout lookups.

- `pkg/worker`
  Worker pool, task processing loop, retry handling, and handler dispatch.

- `pkg/registry`
  In-memory mapping from task names to Go handler functions.

- `pkg/scheduler`
  Polls the scheduled set and promotes due tasks into the pending queue.

- `pkg/reaper`
  Polls the timeout set for expired tasks. The actual timeout recovery path is not finished yet.

- `pkg/config`
  Default queue names and worker/task settings.

## Redis Data Model

Each task is stored as a Redis hash keyed by task ID. Redis also tracks task IDs in:

- `task-queue:pending`
- `task-queue:processing`
- `task-queue:finished`
- `task-queue:dlq`
- `task-set:scheduled`
- `task-set:timeout`

Current task kinds:

- `0`: immediate
- `1`: scheduled
- `2`: cron

Current task stages:

- `pending`
- `in_progress`
- `completed`
- `failed`

## How It Runs Today

- Producer:
  Creates a task with default metadata such as retry count and timeout, then either pushes it to the pending queue or stores it in the scheduled set.

- Worker pool:
  Uses a Redis blocking move from pending to processing, executes the registered handler, and then acknowledges success or requeues/fails the task on error.

- Scheduler:
  Polls the scheduled set, promotes due tasks, and for cron tasks computes the next run before re-adding them to the scheduled set.

- Reaper:
  Reads expired task IDs from the timeout set and logs them. Moving timed-out tasks to the DLQ is still a TODO in code.

## Requirements

- Go `1.25.6`
- Redis `7.x`
- Docker and Docker Compose if you want a local Redis container

## Run Locally

### 1. Start Redis

```sh
docker compose up -d redis
```

### 2. Configure environment

Create `.env` from `.env.example`:

```env
REDIS_URL=localhost:6379
```

### 3. Start the worker

```sh
go run ./cmd/worker
```

Notes:

- This process also starts the scheduler loop.
- The sample worker currently registers only `task_1`.

### 4. Start the reaper

In another terminal:

```sh
go run ./cmd/reaper
```

### 5. Produce tasks

Immediate task:

```sh
go run ./cmd/producer -task task_1 -payload "{\"message\":\"hello\"}" -kind 0
```

Scheduled task:

```sh
go run ./cmd/producer -task task_1 -payload "{\"message\":\"later\"}" -kind 1 -scheduled_at 2026-03-29T12:00:00+05:30
```

Cron task:

```sh
go run ./cmd/producer -task task_1 -payload "{\"message\":\"repeat\"}" -kind 2 -cron "*/10 * * * * *"
```

Notes:

- Scheduled tasks require an RFC3339 timestamp in the future.
- Cron tasks use the `robfig/cron` parser with seconds enabled, so the expression is six-field.

## Current Status

Implemented now:

- Redis-backed enqueue/dequeue flow
- task retries via `Nack`
- finished queue on `Ack`
- scheduled and cron task promotion
- timeout tracking via a Redis sorted set

Coming soon:

- timed-out task recovery and DLQ cleanup in the reaper
- stronger validation and broker-side atomicity improvements
- WASM-based task execution

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
