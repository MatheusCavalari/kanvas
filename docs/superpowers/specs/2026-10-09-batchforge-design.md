# BatchForge — Design Spec

## Overview

**BatchForge** is a batch processing engine built with **Java 25 + Quarkus**, designed to showcase infrastructure-level expertise in chunk-based processing, DAG scheduling, checkpointing, retry, dead-letter handling, partitioning, and real-time observability. Jobs are defined as DAGs of Steps via YAML, each Step following the Reader → Processor → Writer pattern. A React dashboard provides real-time monitoring and a visual DAG builder.

## Goals

- Build a production-grade batch processing engine from scratch (not a wrapper around Spring Batch)
- Showcase Java 25 features: Virtual Threads, Structured Concurrency, Scoped Values, sealed interfaces, records, pattern matching
- Demonstrate chunk-based processing with checkpointing, restart recovery, and item-level fault isolation
- Provide a visual DAG builder and real-time execution monitoring via React dashboard
- Include two showcase use cases: financial reconciliation and log ETL
- Full Docker Compose setup for single-command startup

## Tech Stack

| Component | Technology |
|-----------|------------|
| Engine | Java 25, Quarkus |
| Concurrency | Virtual Threads, Structured Concurrency (`StructuredTaskScope`) |
| Database | PostgreSQL 17 (state, checkpoints, DLQ, metrics history) |
| Cache/Real-time | Redis 7 (locks, rate limiting, real-time metrics pub/sub) |
| Dashboard | React 19, TypeScript, Vite, Tailwind CSS |
| DAG Visualization | React Flow |
| Charts | Recharts |
| Data Fetching | TanStack Query |
| Migrations | Flyway |
| Containerization | Docker, Docker Compose |

## Domain Model

### Core Concepts

| Concept | Description |
|---------|-------------|
| **Job** | Unit of work. DAG of Steps with configuration (name, schedule, parameters) |
| **Step** | DAG node. Contains Reader + Processor + Writer. Processes in chunks |
| **Chunk** | Batch of N items read, processed, and written atomically |
| **Execution** | Instance of a Job run (with status, timestamps, metrics) |
| **Checkpoint** | Progress marker: step + partition + chunk offset. Enables restart |
| **Partition** | Logical data division for intra-step parallelism |
| **Dead Letter** | Items that failed after all retry attempts |

### Execution Lifecycle

```
PENDING → RUNNING → COMPLETED
             ↓
          FAILED → RESTARTING → RUNNING
             ↓
          ABANDONED (after N restarts)
```

### Step Lifecycle

```
WAITING (dependencies) → READY → RUNNING → COMPLETED
                                    ↓
                                  FAILED → RETRYING → RUNNING
                                    ↓
                                  SKIPPED (if skip-on-fail configured)
```

### Domain Entities (Sealed Interfaces + Records)

```java
sealed interface ExecutionStatus permits
    Pending, Running, Completed, Failed, Restarting, Abandoned {}

sealed interface StepStatus permits
    Waiting, Ready, StepRunning, StepCompleted, StepFailed, Retrying, Skipped {}

record JobExecution(
    UUID id, String jobName, ExecutionStatus status,
    Map<String, Object> parameters, Instant startedAt, Instant finishedAt,
    ExecutionMetrics metrics
) {}

record StepExecution(
    UUID id, UUID jobExecutionId, String stepName, StepStatus status,
    int chunksProcessed, int chunksTotal, int itemsFailed,
    Instant startedAt, Instant finishedAt
) {}

record Checkpoint(
    UUID stepExecutionId, String partitionId,
    long offset, Instant savedAt
) {}

record DeadLetterItem(
    UUID id, UUID stepExecutionId, String stepName,
    String itemPayload, String errorMessage, int attemptCount,
    Instant failedAt
) {}
```

## Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        BatchForge Engine                            │
│                                                                     │
│  ┌──────────┐  ┌──────────────┐  ┌────────────┐  ┌──────────────┐ │
│  │  Job      │  │  DAG         │  │  Step      │  │  Checkpoint  │ │
│  │  Registry │──│  Scheduler   │──│  Executor  │──│  Manager     │ │
│  │          │  │  (topo sort) │  │  (VT+SC)   │  │  (Postgres)  │ │
│  └──────────┘  └──────────────┘  └────────────┘  └──────────────┘ │
│        │              │                │                │           │
│  ┌──────────┐  ┌──────────────┐  ┌────────────┐  ┌──────────────┐ │
│  │  YAML    │  │  Partition   │  │  Retry     │  │  Dead Letter │ │
│  │  Parser  │  │  Manager     │  │  Policy    │  │  Store       │ │
│  └──────────┘  └──────────────┘  └────────────┘  └──────────────┘ │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Metrics Collector (throughput, latency, error rate)         │  │
│  │  → Redis (real-time) + Postgres (historical)                │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
        │ REST API + WebSocket
┌───────────────────────────────────────┐
│  React Dashboard                      │
│  • Job list & execution monitoring    │
│  • Real-time progress (WebSocket)     │
│  • Visual DAG builder (React Flow)    │
│  • Dead letter viewer & retry         │
│  • Metrics charts (Recharts)          │
└───────────────────────────────────────┘
```

### Components

**1. Job Registry** — Loads job definitions from YAML, validates DAG (cycles, invalid references), registers in the engine. Hot-reload: watches jobs directory for changes and re-registers.

**2. DAG Scheduler** — Receives a Job execution, performs topological sort of steps, determines which steps can execute in parallel (steps at the same DAG level with no inter-dependencies). Uses Structured Concurrency (`StructuredTaskScope.ShutdownOnFailure`) for coordination.

**3. Step Executor** — Executes an individual Step: instantiates Reader, Processor, Writer. Main loop:
```
while (reader.hasMore()) {
    chunk = reader.read(chunkSize)
    processed = processInParallel(chunk)  // Virtual Threads
    writer.write(processed)
    checkpointManager.save(offset)
}
```
With partitioning: creates N parallel executions of the same step with different ranges.

**4. Checkpoint Manager** — Persists offset per step+partition in Postgres. On restart, queries last checkpoint and passes to Reader for skip-to-offset. Transactional: checkpoint is committed in the same transaction as the chunk write.

**5. Retry Policy** — Configurable per step: max attempts, backoff (fixed/exponential/exponential-jitter), retryable exceptions. Item-level retry: failed item goes to retry queue, chunk continues processing remaining items.

**6. Dead Letter Store** — Items that exceeded max retry go to DLQ (Postgres table). Serialized payload + error + metadata. Dashboard allows inspection, manual retry, or discard.

**7. Partition Manager** — Divides input into logical partitions (ID range, hash, round-robin). Each partition runs in its own Virtual Thread with its own checkpoint.

**8. Metrics Collector** — Collects per step: items/sec, chunks processed, errors, latency p50/p95/p99. Publishes to Redis (Pub/Sub for real-time dashboard) and persists to Postgres (historical).

### SPI (Service Provider Interfaces)

```java
public interface ItemReader<T> {
    void open(ExecutionContext context);
    List<T> read(int chunkSize);
    boolean hasMore();
    void seekTo(long offset);
    void close();
}

public interface ItemProcessor<I, O> {
    O process(I item) throws ProcessingException;
}

public interface ItemWriter<T> {
    void open(ExecutionContext context);
    void write(List<T> items);
    void close();
}

public interface Partitioner {
    List<PartitionRange> partition(ExecutionContext context, int partitionCount);
}
```

### Job Definition YAML

```yaml
name: bank-reconciliation
description: Match transactions against bank statements
schedule: "0 2 * * *"  # 2am daily
parameters:
  accountId: { type: string, required: true }
  date: { type: date, default: "yesterday" }

steps:
  load-transactions:
    reader:
      type: jdbc
      query: "SELECT * FROM transactions WHERE account_id = :accountId AND date = :date"
    processor: com.batchforge.demo.TransactionNormalizer
    writer:
      type: jdbc
      table: staging_transactions
    chunk-size: 1000
    partitions: 4

  load-statements:
    reader:
      type: csv
      path: "/data/statements/${accountId}_${date}.csv"
    processor: com.batchforge.demo.StatementParser
    writer:
      type: jdbc
      table: staging_statements
    chunk-size: 500

  match:
    depends-on: [load-transactions, load-statements]
    reader:
      type: jdbc
      query: "SELECT * FROM staging_transactions WHERE execution_id = :executionId"
    processor: com.batchforge.demo.ReconciliationMatcher
    writer:
      type: jdbc
      table: reconciliation_results
    chunk-size: 500
    retry:
      max-attempts: 3
      backoff: exponential
      base-delay: 1s

  generate-report:
    depends-on: [match]
    reader:
      type: jdbc
      query: "SELECT * FROM reconciliation_results WHERE execution_id = :executionId"
    processor: com.batchforge.demo.ReportGenerator
    writer:
      type: file
      path: "/output/reconciliation_${accountId}_${date}.csv"
    chunk-size: 2000
```

Resulting DAG:
```
load-transactions ──┐
                     ├──► match ──► generate-report
load-statements  ───┘
```

## Data Model (PostgreSQL)

```sql
CREATE TABLE job_definitions (
    name         VARCHAR(255) PRIMARY KEY,
    description  TEXT,
    schedule     VARCHAR(100),
    yaml_hash    VARCHAR(64),
    dag_json     JSONB NOT NULL,
    registered_at TIMESTAMPTZ DEFAULT now(),
    updated_at   TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE job_executions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_name     VARCHAR(255) NOT NULL REFERENCES job_definitions(name),
    status       VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    parameters   JSONB,
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    error_message TEXT,
    restart_count INT DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_job_exec_status ON job_executions(job_name, status);

CREATE TABLE step_executions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_execution_id UUID NOT NULL REFERENCES job_executions(id),
    step_name        VARCHAR(255) NOT NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'WAITING',
    partition_id     VARCHAR(100) DEFAULT 'main',
    chunks_processed INT DEFAULT 0,
    chunks_total     INT,
    items_read       BIGINT DEFAULT 0,
    items_written    BIGINT DEFAULT 0,
    items_failed     BIGINT DEFAULT 0,
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    error_message    TEXT,
    UNIQUE(job_execution_id, step_name, partition_id)
);

CREATE TABLE checkpoints (
    step_execution_id UUID NOT NULL REFERENCES step_executions(id),
    partition_id      VARCHAR(100) NOT NULL DEFAULT 'main',
    reader_offset     BIGINT NOT NULL,
    metadata          JSONB,
    saved_at          TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (step_execution_id, partition_id)
);

CREATE TABLE dead_letter_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    step_execution_id UUID NOT NULL REFERENCES step_executions(id),
    step_name         VARCHAR(255) NOT NULL,
    item_payload      JSONB NOT NULL,
    error_message     TEXT NOT NULL,
    stack_trace       TEXT,
    attempt_count     INT NOT NULL,
    status            VARCHAR(20) DEFAULT 'PENDING',
    failed_at         TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_dlq_status ON dead_letter_items(status, step_name);

CREATE TABLE step_metrics (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    step_execution_id UUID NOT NULL REFERENCES step_executions(id),
    timestamp         TIMESTAMPTZ NOT NULL,
    items_per_second  DOUBLE PRECISION,
    latency_p50_ms    DOUBLE PRECISION,
    latency_p95_ms    DOUBLE PRECISION,
    latency_p99_ms    DOUBLE PRECISION,
    error_count       INT DEFAULT 0,
    memory_used_mb    INT
);
CREATE INDEX idx_metrics_step_ts ON step_metrics(step_execution_id, timestamp);
```

### Redis Usage

| Key Pattern | Type | Purpose |
|-------------|------|---------|
| `bf:exec:{id}:progress` | Hash | Real-time progress per step (items processed, throughput) |
| `bf:exec:{id}:metrics` | Stream | Per-second metrics for dashboard WebSocket |
| `bf:lock:job:{name}` | String + TTL | Lock to prevent concurrent execution of the same job |
| `bf:rate:{step}` | Sorted Set | Rate limiter for steps accessing external APIs |

## REST API

### Jobs
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/jobs` | List registered jobs with last execution status |
| `GET` | `/api/jobs/{name}` | Job details (DAG, config, history) |
| `PUT` | `/api/jobs/{name}` | Register/update job via YAML body |
| `DELETE` | `/api/jobs/{name}` | Remove job from registry |

### Executions
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/jobs/{name}/execute` | Trigger execution (body: parameters) |
| `GET` | `/api/executions` | List executions (filter: job, status, date range) |
| `GET` | `/api/executions/{id}` | Execution details with steps and metrics |
| `POST` | `/api/executions/{id}/restart` | Restart failed execution (resumes from checkpoint) |
| `POST` | `/api/executions/{id}/cancel` | Cancel running execution |

### Steps
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/executions/{id}/steps` | Execution steps with status and progress |
| `GET` | `/api/executions/{id}/steps/{name}/metrics` | Step metrics (throughput, latency) |

### Dead Letter
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/dead-letter` | List DLQ items (filter: step, date) |
| `POST` | `/api/dead-letter/{itemId}/retry` | Manual retry of an item |
| `POST` | `/api/dead-letter/retry-all` | Bulk retry (filter: step) |
| `DELETE` | `/api/dead-letter/{itemId}` | Discard item from DLQ |

### DAG Builder
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/jobs/validate` | Validate job YAML (cycles, invalid refs) |
| `GET` | `/api/readers` | List available built-in readers (jdbc, csv, api) |
| `GET` | `/api/processors` | List registered processors on classpath |
| `GET` | `/api/writers` | List available built-in writers |

### WebSocket

`ws://localhost:8081/ws/executions/{id}` — Real-time event stream:

```json
{"type": "STEP_STARTED", "step": "load-transactions", "timestamp": "..."}
{"type": "CHUNK_COMPLETED", "step": "load-transactions", "partition": "0", "offset": 1000, "itemsPerSec": 4523.7}
{"type": "ITEM_FAILED", "step": "match", "itemId": "txn-8832", "error": "No matching statement", "attempt": 2}
{"type": "STEP_COMPLETED", "step": "load-transactions", "duration": "12.3s", "itemsProcessed": 50000}
{"type": "EXECUTION_COMPLETED", "duration": "45.2s", "totalItems": 150000}
```

## Dashboard (React)

### Stack
- React 19 + TypeScript + Vite + Tailwind CSS
- React Flow (DAG builder/visualizer)
- Recharts (metrics charts)
- TanStack Query (data fetching)

### Screens

**1. Jobs List** — Cards for each job showing last status, next scheduled run, DAG thumbnail.

**2. Execution Detail** — Main screen:
- Visual DAG with status colors (green=completed, blue=running with animation, red=failed, gray=waiting)
- Progress per step (bar + items/sec)
- Real-time metrics updating via WebSocket
- Event timeline
- Action buttons: Restart, Cancel

**3. Visual DAG Builder** — Drag-and-drop editor using React Flow:
- Side palette with available readers/processors/writers
- Drag nodes to canvas, connect with edges
- Per-node form: configure reader type, query, chunk size, retry policy
- Generated YAML preview
- Validate and Save & Register buttons

**4. Dead Letter Viewer** — Filterable table, expandable rows with payload + stack trace, retry/discard per item or in batch.

**5. Metrics Dashboard** — Per execution: throughput over time, latency percentiles, error rate. Comparison between executions of the same job.

## Error Handling and Resilience

### Retry Strategy (per step, YAML-configurable)

```yaml
retry:
  max-attempts: 3
  backoff: exponential       # fixed | exponential | exponential-jitter
  base-delay: 1s
  max-delay: 30s
  retryable-exceptions:
    - java.net.SocketTimeoutException
    - java.sql.SQLTransientException
```

**Item retry flow:**
1. Processor throws exception → engine checks if retryable
2. If retryable and attempts < max → wait backoff delay → retry
3. If non-retryable or attempts >= max → item goes to Dead Letter
4. Chunk continues processing remaining items (item-level fault isolation)

### Checkpointing and Restart

- Checkpoint saved in Postgres **within the same transaction** as the chunk write (atomicity)
- On restart: engine queries last checkpoint per step/partition, calls `reader.seekTo(offset)`
- Completed steps do not re-execute; failed steps resume from last committed chunk
- If step does not support `seekTo` (e.g., paginated API without stable cursor), engine re-processes from step start with idempotency check on writer

### Backpressure

- Chunk size controls natural throughput (smaller chunks = less memory, more checkpoints)
- Optional rate limiter via Redis for steps accessing rate-limited external APIs
- Synchronous chunk loop: writer slower than reader naturally blocks (no unbounded buffer)

### Graceful Shutdown

- `SIGTERM` → engine signals cancellation to all virtual threads
- In-progress chunks complete (no mid-chunk interruption)
- Current progress checkpoint is saved
- Status updated to `FAILED` with "Shutdown requested" message
- Next restart resumes from checkpoint

## Showcase Use Cases

### Use Case 1: Bank Reconciliation

**Scenario**: Process 1M transactions against 800K bank statement records, identify matches, discrepancies, and orphan transactions.

**DAG:**
```
load-transactions (JDBC, 4 partitions) ──┐
                                          ├──► match ──► generate-report
load-statements (CSV)                   ──┘
```

**Steps:**
- `load-transactions`: JDBC reader with partition by ID range. Normalizes values (cents → decimal), standardizes descriptions. Writes to staging table.
- `load-statements`: CSV reader. Parses bank format (date DD/MM/YYYY, comma-separated values). Writes to staging table.
- `match`: JDBC reader from staging. Processor matches by amount + date ±1 day + fuzzy description match. Classifies: MATCHED, UNMATCHED_TRANSACTION, UNMATCHED_STATEMENT.
- `generate-report`: Reads results, generates CSV with summary (total matched, discrepancies, total divergent amount).

**Demonstrates**: partitioning, multi-source ingestion, DAG parallelism, chunk processing.

### Use Case 2: Log ETL

**Scenario**: Ingest application logs (JSON Lines files), enrich with reference data, aggregate by period, persist to analytics table.

**DAG:**
```
ingest-logs (File, JSON Lines) ──► enrich (lookup user/service) ──► aggregate (by hour) ──► persist (JDBC)
```

**Steps:**
- `ingest-logs`: JSON Lines file reader. Parse + schema validation. Invalid items → Dead Letter.
- `enrich`: Reader from staging. Lookup in Redis cache for user and service data. Cache miss → DB query. Write enriched data.
- `aggregate`: Reader from enriched data. Group by service + severity + hour. Calculate counts and percentiles.
- `persist`: Write to Postgres analytics table.

**Demonstrates**: long linear pipeline, dead letter for invalid items, cache lookup in processor, batch aggregation.

## Testing Strategy

| Level | What it tests | Tools |
|-------|---------------|-------|
| Unit | Processors, matchers, parsers individually | JUnit 5 + AssertJ |
| Integration | Reader/Writer with real DB, checkpoint/restart | Testcontainers (Postgres + Redis) |
| Engine | Full job execution, DAG scheduling, parallelism | Testcontainers + test YAML jobs |
| Load | Throughput with 1M+ items, performance metrics | Test job with data generator |

**Critical tests:**
- Checkpoint + restart: kill engine mid-execution, restart, verify resume without duplicates
- Dead letter: inject invalid items, verify DLQ capture without interrupting chunk
- DAG parallelism: job with parallel steps, verify concurrent execution (timing assertions)
- Partition correctness: 4 partitions process disjoint ranges, result equals non-partitioned processing

## Project Structure

```
batchforge/
├── docker-compose.yml
├── Dockerfile                      # Multi-stage: build → JRE slim
├── jobs/                           # YAML job definitions
│   ├── bank-reconciliation.yml
│   └── log-etl.yml
├── src/main/java/com/batchforge/
│   ├── engine/
│   │   ├── core/                   # Job, Step, Execution records + sealed interfaces
│   │   ├── dag/                    # DAG parser, topological sort, scheduler
│   │   ├── executor/               # StepExecutor, ChunkProcessor (VT + SC)
│   │   ├── checkpoint/             # CheckpointManager, restart logic
│   │   ├── partition/              # Partitioner interface + implementations
│   │   ├── retry/                  # RetryPolicy, backoff strategies
│   │   ├── deadletter/             # DeadLetterStore, retry/discard
│   │   └── metrics/                # MetricsCollector, Redis publisher
│   ├── spi/                        # ItemReader, ItemProcessor, ItemWriter, Partitioner
│   ├── readers/                    # JdbcReader, CsvReader, JsonLinesReader, ApiReader
│   ├── writers/                    # JdbcWriter, CsvWriter, FileWriter
│   ├── yaml/                       # YAML parser + validator, hot-reload watcher
│   ├── api/                        # REST endpoints (Quarkus JAX-RS)
│   ├── ws/                         # WebSocket endpoint (execution events)
│   └── demo/                       # Showcase processors (reconciliation, ETL)
│       ├── reconciliation/
│       └── logetl/
├── src/main/resources/
│   ├── application.properties
│   └── db/migration/               # Flyway migrations
├── src/test/java/com/batchforge/
│   ├── engine/                     # Unit + integration tests
│   ├── demo/                       # End-to-end job tests
│   └── load/                       # Load/performance tests
└── dashboard/
    ├── Dockerfile
    ├── package.json
    ├── src/
    │   ├── components/
    │   │   ├── JobList.tsx
    │   │   ├── ExecutionDetail.tsx
    │   │   ├── DagBuilder.tsx       # React Flow based
    │   │   ├── DagViewer.tsx        # Read-only DAG with status colors
    │   │   ├── DeadLetterViewer.tsx
    │   │   ├── MetricsChart.tsx     # Recharts
    │   │   └── StepProgress.tsx
    │   ├── hooks/
    │   │   ├── useExecution.ts      # TanStack Query
    │   │   └── useExecutionStream.ts # WebSocket
    │   ├── api/
    │   └── App.tsx
    └── vite.config.ts
```

## Docker Setup

### docker-compose.yml

```yaml
services:
  batchforge-engine:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
      - "8081:8081"
    environment:
      QUARKUS_DATASOURCE_JDBC_URL: jdbc:postgresql://postgres:5432/batchforge
      QUARKUS_REDIS_HOSTS: redis://redis:6379
      BATCHFORGE_JOBS_DIR: /jobs
    volumes:
      - ./jobs:/jobs
      - ./data:/data
      - ./output:/output
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy

  dashboard:
    build:
      context: ./dashboard
      dockerfile: Dockerfile
    ports:
      - "3000:80"
    environment:
      VITE_API_URL: http://localhost:8080
      VITE_WS_URL: ws://localhost:8081

  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: batchforge
      POSTGRES_USER: batchforge
      POSTGRES_PASSWORD: batchforge
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U batchforge"]
      interval: 5s
      timeout: 3s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
```

### Dockerfile (Multi-stage)

```dockerfile
FROM maven:3.9-eclipse-temurin-25 AS build
WORKDIR /app
COPY pom.xml .
RUN mvn dependency:go-offline
COPY src ./src
RUN mvn package -DskipTests -Dquarkus.package.jar.type=uber-jar

FROM eclipse-temurin:25-jre-alpine
WORKDIR /app
COPY --from=build /app/target/*-runner.jar app.jar
EXPOSE 8080 8081
ENTRYPOINT ["java", "--enable-preview", "-jar", "app.jar"]
```
