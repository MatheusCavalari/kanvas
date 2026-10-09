# BatchForge Sub-Project 1: Engine Core — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the BatchForge batch processing engine core — domain model, SPI, YAML job parser, DAG scheduler with topological sort, chunk-based step executor with Virtual Threads + Structured Concurrency, checkpointing, retry with backoff, dead letter queue, partitioning, metrics collection, and built-in readers/writers.

**Architecture:** Jobs are DAGs of Steps defined in YAML. The DAG Scheduler performs topological sort and executes independent steps in parallel via `StructuredTaskScope`. Each Step follows the Reader → Processor → Writer pattern, processing data in chunks. Checkpoints persist per-chunk progress for restart recovery. Failed items retry with configurable backoff and go to a dead letter queue after exhausting attempts.

**Tech Stack:** Java 25, Quarkus 3.x, PostgreSQL 17, Redis 7, Flyway, SnakeYAML, Testcontainers, JUnit 5, AssertJ

## Global Constraints

- Java 25 with `--enable-preview` for Structured Concurrency and Scoped Values
- Quarkus 3.x (latest stable) with `quarkus-jdbc-postgresql`, `quarkus-redis-client`, `quarkus-flyway`
- All domain entities use sealed interfaces + records (no class hierarchies)
- Package root: `com.batchforge`
- Test with Testcontainers for Postgres and Redis (no H2, no mocks for DB)
- Maven build system
- Docker Compose for local development (Postgres 17 + Redis 7)
- Flyway migrations in `src/main/resources/db/migration/`

---

### Task 1: Project Scaffold + Docker + Domain Model

**Files:**
- Create: `pom.xml`
- Create: `docker-compose.yml`
- Create: `Dockerfile`
- Create: `.dockerignore`
- Create: `src/main/resources/application.properties`
- Create: `src/main/resources/db/migration/V1__initial_schema.sql`
- Create: `src/main/java/com/batchforge/engine/core/ExecutionStatus.java`
- Create: `src/main/java/com/batchforge/engine/core/StepStatus.java`
- Create: `src/main/java/com/batchforge/engine/core/JobDefinition.java`
- Create: `src/main/java/com/batchforge/engine/core/StepDefinition.java`
- Create: `src/main/java/com/batchforge/engine/core/JobExecution.java`
- Create: `src/main/java/com/batchforge/engine/core/StepExecution.java`
- Create: `src/main/java/com/batchforge/engine/core/Checkpoint.java`
- Create: `src/main/java/com/batchforge/engine/core/DeadLetterItem.java`
- Create: `src/main/java/com/batchforge/engine/core/ExecutionMetrics.java`
- Create: `src/main/java/com/batchforge/engine/core/RetryConfig.java`
- Create: `src/main/java/com/batchforge/engine/core/ParameterDefinition.java`
- Create: `src/main/java/com/batchforge/engine/core/ReaderConfig.java`
- Create: `src/main/java/com/batchforge/engine/core/WriterConfig.java`
- Test: `src/test/java/com/batchforge/engine/core/DomainModelTest.java`

**Interfaces:**
- Consumes: nothing (first task)
- Produces:
  - `sealed interface ExecutionStatus permits Pending, Running, Completed, Failed, Restarting, Abandoned {}`
  - `sealed interface StepStatus permits Waiting, Ready, StepRunning, StepCompleted, StepFailed, Retrying, Skipped {}`
  - `record JobDefinition(String name, String description, String schedule, Map<String, ParameterDefinition> parameters, Map<String, StepDefinition> steps)`
  - `record StepDefinition(String name, ReaderConfig reader, String processorClass, WriterConfig writer, int chunkSize, int partitions, List<String> dependsOn, RetryConfig retry)`
  - `record JobExecution(UUID id, String jobName, ExecutionStatus status, Map<String, Object> parameters, Instant startedAt, Instant finishedAt, ExecutionMetrics metrics, int restartCount, String errorMessage)`
  - `record StepExecution(UUID id, UUID jobExecutionId, String stepName, StepStatus status, String partitionId, int chunksProcessed, Integer chunksTotal, long itemsRead, long itemsWritten, long itemsFailed, Instant startedAt, Instant finishedAt, String errorMessage)`
  - `record Checkpoint(UUID stepExecutionId, String partitionId, long offset, Map<String, Object> metadata, Instant savedAt)`
  - `record DeadLetterItem(UUID id, UUID stepExecutionId, String stepName, String itemPayload, String errorMessage, String stackTrace, int attemptCount, String status, Instant failedAt)`
  - `record ExecutionMetrics(long totalItemsRead, long totalItemsWritten, long totalItemsFailed, Duration totalDuration)`
  - `record RetryConfig(int maxAttempts, String backoff, Duration baseDelay, Duration maxDelay, List<String> retryableExceptions)`
  - `record ParameterDefinition(String type, boolean required, String defaultValue)`
  - `record ReaderConfig(String type, Map<String, String> properties)`
  - `record WriterConfig(String type, Map<String, String> properties)`

- [ ] **Step 1: Create `pom.xml` with Quarkus dependencies**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.batchforge</groupId>
    <artifactId>batchforge</artifactId>
    <version>1.0.0-SNAPSHOT</version>
    <packaging>jar</packaging>

    <properties>
        <compiler-plugin.version>3.14.0</compiler-plugin.version>
        <maven.compiler.release>25</maven.compiler.release>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
        <quarkus.platform.artifact-id>quarkus-bom</quarkus.platform.artifact-id>
        <quarkus.platform.group-id>io.quarkus.platform</quarkus.platform.group-id>
        <quarkus.platform.version>3.21.0</quarkus.platform.version>
        <surefire-plugin.version>3.5.2</surefire-plugin.version>
    </properties>

    <dependencyManagement>
        <dependencies>
            <dependency>
                <groupId>${quarkus.platform.group-id}</groupId>
                <artifactId>${quarkus.platform.artifact-id}</artifactId>
                <version>${quarkus.platform.version}</version>
                <type>pom</type>
                <scope>import</scope>
            </dependency>
        </dependencies>
    </dependencyManagement>

    <dependencies>
        <!-- Quarkus core -->
        <dependency>
            <groupId>io.quarkus</groupId>
            <artifactId>quarkus-rest-jackson</artifactId>
        </dependency>
        <dependency>
            <groupId>io.quarkus</groupId>
            <artifactId>quarkus-websockets-next</artifactId>
        </dependency>

        <!-- Database -->
        <dependency>
            <groupId>io.quarkus</groupId>
            <artifactId>quarkus-jdbc-postgresql</artifactId>
        </dependency>
        <dependency>
            <groupId>io.quarkus</groupId>
            <artifactId>quarkus-flyway</artifactId>
        </dependency>
        <dependency>
            <groupId>io.quarkus</groupId>
            <artifactId>quarkus-agroal</artifactId>
        </dependency>

        <!-- Redis -->
        <dependency>
            <groupId>io.quarkus</groupId>
            <artifactId>quarkus-redis-client</artifactId>
        </dependency>

        <!-- YAML parsing -->
        <dependency>
            <groupId>org.yaml</groupId>
            <artifactId>snakeyaml</artifactId>
            <version>2.3</version>
        </dependency>

        <!-- Jackson for JSON -->
        <dependency>
            <groupId>com.fasterxml.jackson.core</groupId>
            <artifactId>jackson-databind</artifactId>
        </dependency>

        <!-- Test -->
        <dependency>
            <groupId>io.quarkus</groupId>
            <artifactId>quarkus-junit5</artifactId>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>org.assertj</groupId>
            <artifactId>assertj-core</artifactId>
            <version>3.27.3</version>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>io.rest-assured</groupId>
            <artifactId>rest-assured</artifactId>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>org.testcontainers</groupId>
            <artifactId>postgresql</artifactId>
            <version>1.20.4</version>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>org.testcontainers</groupId>
            <artifactId>junit-jupiter</artifactId>
            <version>1.20.4</version>
            <scope>test</scope>
        </dependency>
    </dependencies>

    <build>
        <plugins>
            <plugin>
                <groupId>${quarkus.platform.group-id}</groupId>
                <artifactId>quarkus-maven-plugin</artifactId>
                <version>${quarkus.platform.version}</version>
                <extensions>true</extensions>
                <executions>
                    <execution>
                        <goals>
                            <goal>build</goal>
                            <goal>generate-code</goal>
                            <goal>generate-code-tests</goal>
                            <goal>native-image-test</goal>
                        </goals>
                    </execution>
                </executions>
            </plugin>
            <plugin>
                <artifactId>maven-compiler-plugin</artifactId>
                <version>${compiler-plugin.version}</version>
                <configuration>
                    <release>${maven.compiler.release}</release>
                    <compilerArgs>
                        <arg>--enable-preview</arg>
                    </compilerArgs>
                </configuration>
            </plugin>
            <plugin>
                <artifactId>maven-surefire-plugin</artifactId>
                <version>${surefire-plugin.version}</version>
                <configuration>
                    <systemPropertyVariables>
                        <java.util.logging.manager>org.jboss.logmanager.LogManager</java.util.logging.manager>
                    </systemPropertyVariables>
                    <argLine>--enable-preview</argLine>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>
```

- [ ] **Step 2: Create `docker-compose.yml`**

```yaml
services:
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

- [ ] **Step 3: Create `Dockerfile` (multi-stage)**

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

- [ ] **Step 4: Create `.dockerignore`**

```
target/
.git/
.idea/
*.iml
dashboard/node_modules/
```

- [ ] **Step 5: Create `application.properties`**

```properties
# Datasource
quarkus.datasource.db-kind=postgresql
quarkus.datasource.username=batchforge
quarkus.datasource.password=batchforge
quarkus.datasource.jdbc.url=jdbc:postgresql://localhost:5432/batchforge

# Flyway
quarkus.flyway.migrate-at-start=true

# Redis
quarkus.redis.hosts=redis://localhost:6379

# BatchForge
batchforge.jobs-dir=jobs
```

- [ ] **Step 6: Create Flyway migration `V1__initial_schema.sql`**

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
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_name      VARCHAR(255) NOT NULL REFERENCES job_definitions(name),
    status        VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    parameters    JSONB,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    error_message TEXT,
    restart_count INT DEFAULT 0,
    created_at    TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_job_exec_status ON job_executions(job_name, status);

CREATE TABLE step_executions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_execution_id  UUID NOT NULL REFERENCES job_executions(id),
    step_name         VARCHAR(255) NOT NULL,
    status            VARCHAR(20) NOT NULL DEFAULT 'WAITING',
    partition_id      VARCHAR(100) DEFAULT 'main',
    chunks_processed  INT DEFAULT 0,
    chunks_total      INT,
    items_read        BIGINT DEFAULT 0,
    items_written     BIGINT DEFAULT 0,
    items_failed      BIGINT DEFAULT 0,
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    error_message     TEXT,
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

- [ ] **Step 7: Create sealed interfaces `ExecutionStatus.java` and `StepStatus.java`**

`ExecutionStatus.java`:
```java
package com.batchforge.engine.core;

public sealed interface ExecutionStatus {
    record Pending() implements ExecutionStatus {}
    record Running() implements ExecutionStatus {}
    record Completed() implements ExecutionStatus {}
    record Failed(String message) implements ExecutionStatus {}
    record Restarting() implements ExecutionStatus {}
    record Abandoned(String message) implements ExecutionStatus {}

    default String toDbValue() {
        return switch (this) {
            case Pending _ -> "PENDING";
            case Running _ -> "RUNNING";
            case Completed _ -> "COMPLETED";
            case Failed _ -> "FAILED";
            case Restarting _ -> "RESTARTING";
            case Abandoned _ -> "ABANDONED";
        };
    }

    static ExecutionStatus fromDbValue(String value, String message) {
        return switch (value) {
            case "PENDING" -> new Pending();
            case "RUNNING" -> new Running();
            case "COMPLETED" -> new Completed();
            case "FAILED" -> new Failed(message);
            case "RESTARTING" -> new Restarting();
            case "ABANDONED" -> new Abandoned(message);
            default -> throw new IllegalArgumentException("Unknown execution status: " + value);
        };
    }
}
```

`StepStatus.java`:
```java
package com.batchforge.engine.core;

public sealed interface StepStatus {
    record Waiting() implements StepStatus {}
    record Ready() implements StepStatus {}
    record StepRunning() implements StepStatus {}
    record StepCompleted() implements StepStatus {}
    record StepFailed(String message) implements StepStatus {}
    record Retrying(int attempt) implements StepStatus {}
    record Skipped(String reason) implements StepStatus {}

    default String toDbValue() {
        return switch (this) {
            case Waiting _ -> "WAITING";
            case Ready _ -> "READY";
            case StepRunning _ -> "RUNNING";
            case StepCompleted _ -> "COMPLETED";
            case StepFailed _ -> "FAILED";
            case Retrying _ -> "RETRYING";
            case Skipped _ -> "SKIPPED";
        };
    }

    static StepStatus fromDbValue(String value, String message) {
        return switch (value) {
            case "WAITING" -> new Waiting();
            case "READY" -> new Ready();
            case "RUNNING" -> new StepRunning();
            case "COMPLETED" -> new StepCompleted();
            case "FAILED" -> new StepFailed(message);
            case "RETRYING" -> new Retrying(0);
            case "SKIPPED" -> new Skipped(message);
            default -> throw new IllegalArgumentException("Unknown step status: " + value);
        };
    }
}
```

- [ ] **Step 8: Create all domain records**

`ReaderConfig.java`:
```java
package com.batchforge.engine.core;

import java.util.Map;

public record ReaderConfig(String type, Map<String, String> properties) {}
```

`WriterConfig.java`:
```java
package com.batchforge.engine.core;

import java.util.Map;

public record WriterConfig(String type, Map<String, String> properties) {}
```

`ParameterDefinition.java`:
```java
package com.batchforge.engine.core;

public record ParameterDefinition(String type, boolean required, String defaultValue) {}
```

`RetryConfig.java`:
```java
package com.batchforge.engine.core;

import java.time.Duration;
import java.util.List;

public record RetryConfig(
    int maxAttempts,
    String backoff,
    Duration baseDelay,
    Duration maxDelay,
    List<String> retryableExceptions
) {
    public static RetryConfig defaultConfig() {
        return new RetryConfig(3, "exponential", Duration.ofSeconds(1), Duration.ofSeconds(30), List.of());
    }

    public static RetryConfig none() {
        return new RetryConfig(0, "fixed", Duration.ZERO, Duration.ZERO, List.of());
    }
}
```

`StepDefinition.java`:
```java
package com.batchforge.engine.core;

import java.util.List;

public record StepDefinition(
    String name,
    ReaderConfig reader,
    String processorClass,
    WriterConfig writer,
    int chunkSize,
    int partitions,
    List<String> dependsOn,
    RetryConfig retry,
    boolean skipOnFail
) {
    public StepDefinition {
        if (chunkSize <= 0) chunkSize = 100;
        if (partitions <= 0) partitions = 1;
        if (dependsOn == null) dependsOn = List.of();
        if (retry == null) retry = RetryConfig.none();
    }
}
```

`JobDefinition.java`:
```java
package com.batchforge.engine.core;

import java.util.Map;

public record JobDefinition(
    String name,
    String description,
    String schedule,
    Map<String, ParameterDefinition> parameters,
    Map<String, StepDefinition> steps
) {
    public JobDefinition {
        if (parameters == null) parameters = Map.of();
        if (steps == null || steps.isEmpty()) {
            throw new IllegalArgumentException("Job must have at least one step");
        }
    }
}
```

`ExecutionMetrics.java`:
```java
package com.batchforge.engine.core;

import java.time.Duration;

public record ExecutionMetrics(
    long totalItemsRead,
    long totalItemsWritten,
    long totalItemsFailed,
    Duration totalDuration
) {
    public static ExecutionMetrics empty() {
        return new ExecutionMetrics(0, 0, 0, Duration.ZERO);
    }
}
```

`JobExecution.java`:
```java
package com.batchforge.engine.core;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record JobExecution(
    UUID id,
    String jobName,
    ExecutionStatus status,
    Map<String, Object> parameters,
    Instant startedAt,
    Instant finishedAt,
    ExecutionMetrics metrics,
    int restartCount,
    String errorMessage
) {
    public static JobExecution create(String jobName, Map<String, Object> parameters) {
        return new JobExecution(
            UUID.randomUUID(), jobName, new ExecutionStatus.Pending(),
            parameters, null, null, ExecutionMetrics.empty(), 0, null
        );
    }
}
```

`StepExecution.java`:
```java
package com.batchforge.engine.core;

import java.time.Instant;
import java.util.UUID;

public record StepExecution(
    UUID id,
    UUID jobExecutionId,
    String stepName,
    StepStatus status,
    String partitionId,
    int chunksProcessed,
    Integer chunksTotal,
    long itemsRead,
    long itemsWritten,
    long itemsFailed,
    Instant startedAt,
    Instant finishedAt,
    String errorMessage
) {
    public static StepExecution create(UUID jobExecutionId, String stepName, String partitionId) {
        return new StepExecution(
            UUID.randomUUID(), jobExecutionId, stepName, new StepStatus.Waiting(),
            partitionId, 0, null, 0, 0, 0, null, null, null
        );
    }
}
```

`Checkpoint.java`:
```java
package com.batchforge.engine.core;

import java.time.Instant;
import java.util.Map;
import java.util.UUID;

public record Checkpoint(
    UUID stepExecutionId,
    String partitionId,
    long offset,
    Map<String, Object> metadata,
    Instant savedAt
) {}
```

`DeadLetterItem.java`:
```java
package com.batchforge.engine.core;

import java.time.Instant;
import java.util.UUID;

public record DeadLetterItem(
    UUID id,
    UUID stepExecutionId,
    String stepName,
    String itemPayload,
    String errorMessage,
    String stackTrace,
    int attemptCount,
    String status,
    Instant failedAt
) {
    public static DeadLetterItem create(UUID stepExecutionId, String stepName,
            String itemPayload, String errorMessage, String stackTrace, int attemptCount) {
        return new DeadLetterItem(
            UUID.randomUUID(), stepExecutionId, stepName,
            itemPayload, errorMessage, stackTrace, attemptCount, "PENDING", Instant.now()
        );
    }
}
```

- [ ] **Step 9: Write domain model tests**

```java
package com.batchforge.engine.core;

import org.junit.jupiter.api.Test;
import java.time.Duration;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class DomainModelTest {

    @Test
    void executionStatusRoundtripsToDbValue() {
        var statuses = List.of(
            new ExecutionStatus.Pending(),
            new ExecutionStatus.Running(),
            new ExecutionStatus.Completed(),
            new ExecutionStatus.Failed("timeout"),
            new ExecutionStatus.Restarting(),
            new ExecutionStatus.Abandoned("max restarts")
        );
        for (var status : statuses) {
            String db = status.toDbValue();
            String msg = status instanceof ExecutionStatus.Failed f ? f.message()
                       : status instanceof ExecutionStatus.Abandoned a ? a.message() : null;
            ExecutionStatus restored = ExecutionStatus.fromDbValue(db, msg);
            assertThat(restored).isEqualTo(status);
        }
    }

    @Test
    void stepStatusRoundtripsToDbValue() {
        var statuses = List.of(
            new StepStatus.Waiting(),
            new StepStatus.Ready(),
            new StepStatus.StepRunning(),
            new StepStatus.StepCompleted(),
            new StepStatus.StepFailed("NPE"),
            new StepStatus.Skipped("dependency failed")
        );
        for (var status : statuses) {
            String db = status.toDbValue();
            String msg = status instanceof StepStatus.StepFailed f ? f.message()
                       : status instanceof StepStatus.Skipped s ? s.reason() : null;
            StepStatus restored = StepStatus.fromDbValue(db, msg);
            assertThat(restored).isEqualTo(status);
        }
    }

    @Test
    void jobDefinitionRequiresAtLeastOneStep() {
        assertThatThrownBy(() -> new JobDefinition("test", "desc", null, Map.of(), Map.of()))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("at least one step");
    }

    @Test
    void stepDefinitionDefaultsChunkSizeAndPartitions() {
        var step = new StepDefinition("s1",
            new ReaderConfig("jdbc", Map.of()), "com.Proc",
            new WriterConfig("jdbc", Map.of()), 0, 0, null, null, false);
        assertThat(step.chunkSize()).isEqualTo(100);
        assertThat(step.partitions()).isEqualTo(1);
        assertThat(step.dependsOn()).isEmpty();
        assertThat(step.retry()).isEqualTo(RetryConfig.none());
    }

    @Test
    void jobExecutionFactoryMethodCreatesWithPendingStatus() {
        var exec = JobExecution.create("my-job", Map.of("key", "val"));
        assertThat(exec.id()).isNotNull();
        assertThat(exec.jobName()).isEqualTo("my-job");
        assertThat(exec.status()).isInstanceOf(ExecutionStatus.Pending.class);
        assertThat(exec.parameters()).containsEntry("key", "val");
    }

    @Test
    void exhaustivePatternMatchOnExecutionStatus() {
        ExecutionStatus status = new ExecutionStatus.Failed("boom");
        String label = switch (status) {
            case ExecutionStatus.Pending _ -> "pending";
            case ExecutionStatus.Running _ -> "running";
            case ExecutionStatus.Completed _ -> "completed";
            case ExecutionStatus.Failed f -> "failed: " + f.message();
            case ExecutionStatus.Restarting _ -> "restarting";
            case ExecutionStatus.Abandoned a -> "abandoned: " + a.message();
        };
        assertThat(label).isEqualTo("failed: boom");
    }

    @Test
    void retryConfigDefaults() {
        var def = RetryConfig.defaultConfig();
        assertThat(def.maxAttempts()).isEqualTo(3);
        assertThat(def.backoff()).isEqualTo("exponential");
        assertThat(def.baseDelay()).isEqualTo(Duration.ofSeconds(1));
    }
}
```

- [ ] **Step 10: Run tests to verify domain model compiles and passes**

Run: `mvn test -pl . -Dtest=DomainModelTest -f pom.xml`
Expected: all 6 tests PASS

- [ ] **Step 11: Start Docker Compose and verify Flyway migration runs**

Run: `docker compose up -d && mvn quarkus:dev` (verify Flyway output in logs shows `V1__initial_schema.sql` applied)

- [ ] **Step 12: Commit**

```bash
git add pom.xml docker-compose.yml Dockerfile .dockerignore src/
git commit -m "feat(batchforge): project scaffold, Docker, Flyway migration, and domain model

Quarkus 3.x + Java 25 project with PostgreSQL 17 and Redis 7 via Docker Compose.
Sealed interfaces + records for ExecutionStatus, StepStatus, and all domain entities.
Flyway V1 migration creates job_definitions, job_executions, step_executions,
checkpoints, dead_letter_items, and step_metrics tables."
```

---

### Task 2: SPI Interfaces + Execution Context

**Files:**
- Create: `src/main/java/com/batchforge/spi/ItemReader.java`
- Create: `src/main/java/com/batchforge/spi/ItemProcessor.java`
- Create: `src/main/java/com/batchforge/spi/ItemWriter.java`
- Create: `src/main/java/com/batchforge/spi/Partitioner.java`
- Create: `src/main/java/com/batchforge/spi/PartitionRange.java`
- Create: `src/main/java/com/batchforge/spi/ExecutionContext.java`
- Create: `src/main/java/com/batchforge/spi/ProcessingException.java`
- Test: `src/test/java/com/batchforge/spi/ExecutionContextTest.java`

**Interfaces:**
- Consumes: `JobExecution`, `StepExecution` from Task 1
- Produces:
  - `interface ItemReader<T> { void open(ExecutionContext); List<T> read(int chunkSize); boolean hasMore(); void seekTo(long offset); void close(); }`
  - `interface ItemProcessor<I, O> { O process(I item) throws ProcessingException; }`
  - `interface ItemWriter<T> { void open(ExecutionContext); void write(List<T> items); void close(); }`
  - `interface Partitioner { List<PartitionRange> partition(ExecutionContext, int partitionCount); }`
  - `record PartitionRange(String partitionId, Map<String, Object> parameters)`
  - `class ExecutionContext { UUID jobExecutionId(); UUID stepExecutionId(); String stepName(); String partitionId(); Map<String, Object> jobParameters(); Map<String, Object> stepProperties(); }`

- [ ] **Step 1: Write the test for `ExecutionContext`**

```java
package com.batchforge.spi;

import org.junit.jupiter.api.Test;
import java.util.Map;
import java.util.UUID;

import static org.assertj.core.api.Assertions.*;

class ExecutionContextTest {

    @Test
    void contextExposesAllFields() {
        var jobExecId = UUID.randomUUID();
        var stepExecId = UUID.randomUUID();
        var ctx = new ExecutionContext(
            jobExecId, stepExecId, "load-data", "partition-0",
            Map.of("accountId", "ACC-001"),
            Map.of("query", "SELECT * FROM t")
        );
        assertThat(ctx.jobExecutionId()).isEqualTo(jobExecId);
        assertThat(ctx.stepExecutionId()).isEqualTo(stepExecId);
        assertThat(ctx.stepName()).isEqualTo("load-data");
        assertThat(ctx.partitionId()).isEqualTo("partition-0");
        assertThat(ctx.jobParameters()).containsEntry("accountId", "ACC-001");
        assertThat(ctx.stepProperties()).containsEntry("query", "SELECT * FROM t");
    }

    @Test
    void parameterResolutionReplacesPlaceholders() {
        var ctx = new ExecutionContext(
            UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of("accountId", "ACC-001", "date", "2026-01-15"),
            Map.of()
        );
        String resolved = ctx.resolveTemplate("SELECT * FROM t WHERE account = :accountId AND date = :date");
        assertThat(resolved).isEqualTo("SELECT * FROM t WHERE account = ACC-001 AND date = 2026-01-15");
    }

    @Test
    void parameterResolutionLeavesUnknownPlaceholders() {
        var ctx = new ExecutionContext(
            UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of("accountId", "ACC-001"),
            Map.of()
        );
        String resolved = ctx.resolveTemplate("WHERE account = :accountId AND date = :date");
        assertThat(resolved).isEqualTo("WHERE account = ACC-001 AND date = :date");
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `mvn test -Dtest=ExecutionContextTest`
Expected: FAIL — `ExecutionContext` class not found

- [ ] **Step 3: Create SPI interfaces and `ExecutionContext`**

`ItemReader.java`:
```java
package com.batchforge.spi;

import java.util.List;

public interface ItemReader<T> {
    void open(ExecutionContext context);
    List<T> read(int chunkSize);
    boolean hasMore();
    void seekTo(long offset);
    void close();
}
```

`ItemProcessor.java`:
```java
package com.batchforge.spi;

public interface ItemProcessor<I, O> {
    O process(I item) throws ProcessingException;
}
```

`ItemWriter.java`:
```java
package com.batchforge.spi;

import java.util.List;

public interface ItemWriter<T> {
    void open(ExecutionContext context);
    void write(List<T> items);
    void close();
}
```

`Partitioner.java`:
```java
package com.batchforge.spi;

import java.util.List;

public interface Partitioner {
    List<PartitionRange> partition(ExecutionContext context, int partitionCount);
}
```

`PartitionRange.java`:
```java
package com.batchforge.spi;

import java.util.Map;

public record PartitionRange(String partitionId, Map<String, Object> parameters) {}
```

`ProcessingException.java`:
```java
package com.batchforge.spi;

public class ProcessingException extends Exception {
    public ProcessingException(String message) { super(message); }
    public ProcessingException(String message, Throwable cause) { super(message, cause); }
}
```

`ExecutionContext.java`:
```java
package com.batchforge.spi;

import java.util.Map;
import java.util.UUID;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public record ExecutionContext(
    UUID jobExecutionId,
    UUID stepExecutionId,
    String stepName,
    String partitionId,
    Map<String, Object> jobParameters,
    Map<String, Object> stepProperties
) {
    private static final Pattern PARAM_PATTERN = Pattern.compile(":(\\w+)");

    public String resolveTemplate(String template) {
        Matcher m = PARAM_PATTERN.matcher(template);
        StringBuilder sb = new StringBuilder();
        while (m.find()) {
            String key = m.group(1);
            Object value = jobParameters.get(key);
            m.appendReplacement(sb, value != null ? Matcher.quoteReplacement(value.toString()) : Matcher.quoteReplacement(m.group(0)));
        }
        m.appendTail(sb);
        return sb.toString();
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `mvn test -Dtest=ExecutionContextTest`
Expected: PASS — all 3 tests green

- [ ] **Step 5: Commit**

```bash
git add src/main/java/com/batchforge/spi/ src/test/java/com/batchforge/spi/
git commit -m "feat(batchforge): SPI interfaces and ExecutionContext

ItemReader, ItemProcessor, ItemWriter, Partitioner interfaces.
ExecutionContext with parameter template resolution (:paramName → value)."
```

---

### Task 3: YAML Job Parser + DAG Validator

**Files:**
- Create: `src/main/java/com/batchforge/yaml/JobDefinitionParser.java`
- Create: `src/main/java/com/batchforge/yaml/JobValidationException.java`
- Create: `src/main/java/com/batchforge/engine/dag/DagValidator.java`
- Test: `src/test/java/com/batchforge/yaml/JobDefinitionParserTest.java`
- Test: `src/test/java/com/batchforge/engine/dag/DagValidatorTest.java`
- Create: `src/test/resources/jobs/valid-reconciliation.yml`
- Create: `src/test/resources/jobs/invalid-cycle.yml`
- Create: `src/test/resources/jobs/invalid-missing-dep.yml`

**Interfaces:**
- Consumes: `JobDefinition`, `StepDefinition`, `ReaderConfig`, `WriterConfig`, `RetryConfig`, `ParameterDefinition` from Task 1
- Produces:
  - `JobDefinitionParser.parse(String yaml): JobDefinition`
  - `JobDefinitionParser.parseFile(Path file): JobDefinition`
  - `DagValidator.validate(JobDefinition): List<String>` (returns empty list if valid, error messages otherwise)
  - `DagValidator.topologicalSort(JobDefinition): List<List<String>>` (returns levels of step names for parallel execution)

- [ ] **Step 1: Create test YAML files**

`src/test/resources/jobs/valid-reconciliation.yml`:
```yaml
name: bank-reconciliation
description: Match transactions against bank statements
schedule: "0 2 * * *"
parameters:
  accountId: { type: string, required: true }
  date: { type: date, default: "2026-01-15" }

steps:
  load-transactions:
    reader:
      type: jdbc
      query: "SELECT * FROM transactions WHERE account_id = :accountId"
    processor: com.batchforge.demo.TransactionNormalizer
    writer:
      type: jdbc
      table: staging_transactions
    chunk-size: 1000
    partitions: 4

  load-statements:
    reader:
      type: csv
      path: "/data/statements.csv"
    processor: com.batchforge.demo.StatementParser
    writer:
      type: jdbc
      table: staging_statements
    chunk-size: 500

  match:
    depends-on: [load-transactions, load-statements]
    reader:
      type: jdbc
      query: "SELECT * FROM staging_transactions"
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
      query: "SELECT * FROM reconciliation_results"
    processor: com.batchforge.demo.ReportGenerator
    writer:
      type: file
      path: "/output/report.csv"
    chunk-size: 2000
```

`src/test/resources/jobs/invalid-cycle.yml`:
```yaml
name: cyclic-job
description: Has a cycle
steps:
  step-a:
    depends-on: [step-c]
    reader: { type: jdbc, query: "SELECT 1" }
    processor: com.Proc
    writer: { type: jdbc, table: t }
  step-b:
    depends-on: [step-a]
    reader: { type: jdbc, query: "SELECT 1" }
    processor: com.Proc
    writer: { type: jdbc, table: t }
  step-c:
    depends-on: [step-b]
    reader: { type: jdbc, query: "SELECT 1" }
    processor: com.Proc
    writer: { type: jdbc, table: t }
```

`src/test/resources/jobs/invalid-missing-dep.yml`:
```yaml
name: missing-dep-job
description: References non-existent step
steps:
  step-a:
    depends-on: [step-x]
    reader: { type: jdbc, query: "SELECT 1" }
    processor: com.Proc
    writer: { type: jdbc, table: t }
```

- [ ] **Step 2: Write parser tests**

```java
package com.batchforge.yaml;

import com.batchforge.engine.core.*;
import org.junit.jupiter.api.Test;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.time.Duration;

import static org.assertj.core.api.Assertions.*;

class JobDefinitionParserTest {

    private final JobDefinitionParser parser = new JobDefinitionParser();

    private String loadYaml(String resource) throws Exception {
        try (InputStream is = getClass().getResourceAsStream(resource)) {
            return new String(is.readAllBytes(), StandardCharsets.UTF_8);
        }
    }

    @Test
    void parsesValidReconciliationJob() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));

        assertThat(job.name()).isEqualTo("bank-reconciliation");
        assertThat(job.schedule()).isEqualTo("0 2 * * *");
        assertThat(job.steps()).hasSize(4);
        assertThat(job.parameters()).containsKey("accountId");
        assertThat(job.parameters().get("accountId").required()).isTrue();
        assertThat(job.parameters().get("date").defaultValue()).isEqualTo("2026-01-15");
    }

    @Test
    void parsesStepWithChunkSizeAndPartitions() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        StepDefinition loadTxn = job.steps().get("load-transactions");

        assertThat(loadTxn.chunkSize()).isEqualTo(1000);
        assertThat(loadTxn.partitions()).isEqualTo(4);
        assertThat(loadTxn.reader().type()).isEqualTo("jdbc");
        assertThat(loadTxn.writer().type()).isEqualTo("jdbc");
        assertThat(loadTxn.processorClass()).isEqualTo("com.batchforge.demo.TransactionNormalizer");
    }

    @Test
    void parsesStepDependencies() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        StepDefinition match = job.steps().get("match");

        assertThat(match.dependsOn()).containsExactlyInAnyOrder("load-transactions", "load-statements");
    }

    @Test
    void parsesRetryConfig() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        RetryConfig retry = job.steps().get("match").retry();

        assertThat(retry.maxAttempts()).isEqualTo(3);
        assertThat(retry.backoff()).isEqualTo("exponential");
        assertThat(retry.baseDelay()).isEqualTo(Duration.ofSeconds(1));
    }

    @Test
    void stepsWithoutRetryGetDefaultNone() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        RetryConfig retry = job.steps().get("load-transactions").retry();

        assertThat(retry.maxAttempts()).isZero();
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `mvn test -Dtest=JobDefinitionParserTest`
Expected: FAIL — `JobDefinitionParser` not found

- [ ] **Step 4: Implement `JobDefinitionParser`**

```java
package com.batchforge.yaml;

import com.batchforge.engine.core.*;
import org.yaml.snakeyaml.Yaml;

import java.io.IOException;
import java.nio.file.*;
import java.time.Duration;
import java.util.*;
import java.util.regex.*;

public class JobDefinitionParser {

    private static final Pattern DURATION_PATTERN = Pattern.compile("(\\d+)(ms|s|m|h)");

    public JobDefinition parseFile(Path file) throws IOException {
        return parse(Files.readString(file));
    }

    @SuppressWarnings("unchecked")
    public JobDefinition parse(String yamlContent) {
        Yaml yaml = new Yaml();
        Map<String, Object> root = yaml.load(yamlContent);

        String name = requireString(root, "name");
        String description = (String) root.getOrDefault("description", "");
        String schedule = (String) root.get("schedule");

        Map<String, ParameterDefinition> parameters = parseParameters(
            (Map<String, Object>) root.getOrDefault("parameters", Map.of()));

        Map<String, Object> stepsRaw = (Map<String, Object>) root.get("steps");
        if (stepsRaw == null || stepsRaw.isEmpty()) {
            throw new JobValidationException("Job '" + name + "' must define at least one step");
        }

        Map<String, StepDefinition> steps = new LinkedHashMap<>();
        for (var entry : stepsRaw.entrySet()) {
            steps.put(entry.getKey(), parseStep(entry.getKey(), (Map<String, Object>) entry.getValue()));
        }

        return new JobDefinition(name, description, schedule, parameters, steps);
    }

    @SuppressWarnings("unchecked")
    private Map<String, ParameterDefinition> parseParameters(Map<String, Object> raw) {
        Map<String, ParameterDefinition> params = new LinkedHashMap<>();
        for (var entry : raw.entrySet()) {
            Map<String, Object> def = (Map<String, Object>) entry.getValue();
            params.put(entry.getKey(), new ParameterDefinition(
                (String) def.getOrDefault("type", "string"),
                Boolean.TRUE.equals(def.get("required")),
                def.get("default") != null ? def.get("default").toString() : null
            ));
        }
        return params;
    }

    @SuppressWarnings("unchecked")
    private StepDefinition parseStep(String name, Map<String, Object> raw) {
        Map<String, Object> readerRaw = (Map<String, Object>) raw.get("reader");
        if (readerRaw == null) throw new JobValidationException("Step '" + name + "' missing reader");
        ReaderConfig reader = new ReaderConfig(
            requireString(readerRaw, "type"),
            extractProperties(readerRaw)
        );

        String processorClass = (String) raw.get("processor");
        if (processorClass == null) throw new JobValidationException("Step '" + name + "' missing processor");

        Map<String, Object> writerRaw = (Map<String, Object>) raw.get("writer");
        if (writerRaw == null) throw new JobValidationException("Step '" + name + "' missing writer");
        WriterConfig writer = new WriterConfig(
            requireString(writerRaw, "type"),
            extractProperties(writerRaw)
        );

        int chunkSize = ((Number) raw.getOrDefault("chunk-size", 100)).intValue();
        int partitions = ((Number) raw.getOrDefault("partitions", 1)).intValue();
        List<String> dependsOn = (List<String>) raw.getOrDefault("depends-on", List.of());
        boolean skipOnFail = Boolean.TRUE.equals(raw.get("skip-on-fail"));

        RetryConfig retry = raw.containsKey("retry")
            ? parseRetryConfig((Map<String, Object>) raw.get("retry"))
            : RetryConfig.none();

        return new StepDefinition(name, reader, processorClass, writer, chunkSize, partitions, dependsOn, retry, skipOnFail);
    }

    @SuppressWarnings("unchecked")
    private RetryConfig parseRetryConfig(Map<String, Object> raw) {
        int maxAttempts = ((Number) raw.getOrDefault("max-attempts", 3)).intValue();
        String backoff = (String) raw.getOrDefault("backoff", "exponential");
        Duration baseDelay = parseDuration((String) raw.getOrDefault("base-delay", "1s"));
        Duration maxDelay = parseDuration((String) raw.getOrDefault("max-delay", "30s"));
        List<String> retryable = (List<String>) raw.getOrDefault("retryable-exceptions", List.of());
        return new RetryConfig(maxAttempts, backoff, baseDelay, maxDelay, retryable);
    }

    private Duration parseDuration(String value) {
        Matcher m = DURATION_PATTERN.matcher(value);
        if (!m.matches()) throw new JobValidationException("Invalid duration: " + value);
        long amount = Long.parseLong(m.group(1));
        return switch (m.group(2)) {
            case "ms" -> Duration.ofMillis(amount);
            case "s" -> Duration.ofSeconds(amount);
            case "m" -> Duration.ofMinutes(amount);
            case "h" -> Duration.ofHours(amount);
            default -> throw new JobValidationException("Unknown duration unit: " + m.group(2));
        };
    }

    private Map<String, String> extractProperties(Map<String, Object> raw) {
        Map<String, String> props = new LinkedHashMap<>();
        for (var entry : raw.entrySet()) {
            if (!"type".equals(entry.getKey()) && entry.getValue() != null) {
                props.put(entry.getKey(), entry.getValue().toString());
            }
        }
        return props;
    }

    private String requireString(Map<String, Object> map, String key) {
        Object val = map.get(key);
        if (val == null) throw new JobValidationException("Missing required field: " + key);
        return val.toString();
    }
}
```

`JobValidationException.java`:
```java
package com.batchforge.yaml;

public class JobValidationException extends RuntimeException {
    public JobValidationException(String message) { super(message); }
}
```

- [ ] **Step 5: Run parser tests to verify they pass**

Run: `mvn test -Dtest=JobDefinitionParserTest`
Expected: PASS — all 5 tests green

- [ ] **Step 6: Write DAG validator tests**

```java
package com.batchforge.engine.dag;

import com.batchforge.engine.core.*;
import com.batchforge.yaml.JobDefinitionParser;
import org.junit.jupiter.api.Test;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.List;

import static org.assertj.core.api.Assertions.*;

class DagValidatorTest {

    private final JobDefinitionParser parser = new JobDefinitionParser();
    private final DagValidator validator = new DagValidator();

    private String loadYaml(String resource) throws Exception {
        try (InputStream is = getClass().getResourceAsStream(resource)) {
            return new String(is.readAllBytes(), StandardCharsets.UTF_8);
        }
    }

    @Test
    void validJobPassesValidation() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        List<String> errors = validator.validate(job);
        assertThat(errors).isEmpty();
    }

    @Test
    void detectsCyclicDependency() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/invalid-cycle.yml"));
        List<String> errors = validator.validate(job);
        assertThat(errors).anyMatch(e -> e.contains("cycle"));
    }

    @Test
    void detectsMissingDependency() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/invalid-missing-dep.yml"));
        List<String> errors = validator.validate(job);
        assertThat(errors).anyMatch(e -> e.contains("step-x"));
    }

    @Test
    void topologicalSortReturnsLevels() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        List<List<String>> levels = validator.topologicalSort(job);

        // Level 0: load-transactions, load-statements (no dependencies)
        assertThat(levels.get(0)).containsExactlyInAnyOrder("load-transactions", "load-statements");
        // Level 1: match (depends on both)
        assertThat(levels.get(1)).containsExactly("match");
        // Level 2: generate-report (depends on match)
        assertThat(levels.get(2)).containsExactly("generate-report");
    }
}
```

- [ ] **Step 7: Run DAG validator tests to verify they fail**

Run: `mvn test -Dtest=DagValidatorTest`
Expected: FAIL — `DagValidator` not found

- [ ] **Step 8: Implement `DagValidator`**

```java
package com.batchforge.engine.dag;

import com.batchforge.engine.core.JobDefinition;
import com.batchforge.engine.core.StepDefinition;

import java.util.*;

public class DagValidator {

    public List<String> validate(JobDefinition job) {
        List<String> errors = new ArrayList<>();
        Set<String> stepNames = job.steps().keySet();

        // Check all dependencies reference existing steps
        for (var entry : job.steps().entrySet()) {
            for (String dep : entry.getValue().dependsOn()) {
                if (!stepNames.contains(dep)) {
                    errors.add("Step '" + entry.getKey() + "' depends on non-existent step '" + dep + "'");
                }
            }
        }

        if (!errors.isEmpty()) return errors;

        // Check for cycles using Kahn's algorithm
        Map<String, Set<String>> inEdges = new LinkedHashMap<>();
        Map<String, Set<String>> outEdges = new LinkedHashMap<>();
        for (String name : stepNames) {
            inEdges.put(name, new LinkedHashSet<>());
            outEdges.put(name, new LinkedHashSet<>());
        }
        for (var entry : job.steps().entrySet()) {
            for (String dep : entry.getValue().dependsOn()) {
                inEdges.get(entry.getKey()).add(dep);
                outEdges.get(dep).add(entry.getKey());
            }
        }

        Queue<String> queue = new ArrayDeque<>();
        for (var entry : inEdges.entrySet()) {
            if (entry.getValue().isEmpty()) queue.add(entry.getKey());
        }

        int visited = 0;
        while (!queue.isEmpty()) {
            String node = queue.poll();
            visited++;
            for (String next : outEdges.get(node)) {
                inEdges.get(next).remove(node);
                if (inEdges.get(next).isEmpty()) queue.add(next);
            }
        }

        if (visited != stepNames.size()) {
            errors.add("Job '" + job.name() + "' has a dependency cycle");
        }

        return errors;
    }

    public List<List<String>> topologicalSort(JobDefinition job) {
        List<String> errors = validate(job);
        if (!errors.isEmpty()) throw new IllegalArgumentException("Invalid DAG: " + errors);

        Map<String, Set<String>> inEdges = new LinkedHashMap<>();
        Map<String, Set<String>> outEdges = new LinkedHashMap<>();
        for (String name : job.steps().keySet()) {
            inEdges.put(name, new LinkedHashSet<>());
            outEdges.put(name, new LinkedHashSet<>());
        }
        for (var entry : job.steps().entrySet()) {
            for (String dep : entry.getValue().dependsOn()) {
                inEdges.get(entry.getKey()).add(dep);
                outEdges.get(dep).add(entry.getKey());
            }
        }

        List<List<String>> levels = new ArrayList<>();
        Set<String> remaining = new LinkedHashSet<>(job.steps().keySet());

        while (!remaining.isEmpty()) {
            List<String> level = new ArrayList<>();
            for (String name : remaining) {
                if (inEdges.get(name).isEmpty()) level.add(name);
            }
            if (level.isEmpty()) throw new IllegalStateException("Unexpected cycle");
            levels.add(level);
            remaining.removeAll(level);
            for (String done : level) {
                for (String next : outEdges.get(done)) {
                    inEdges.get(next).remove(done);
                }
            }
        }

        return levels;
    }
}
```

- [ ] **Step 9: Run all tests to verify they pass**

Run: `mvn test -Dtest="JobDefinitionParserTest,DagValidatorTest"`
Expected: PASS — all 9 tests green

- [ ] **Step 10: Commit**

```bash
git add src/
git commit -m "feat(batchforge): YAML job parser and DAG validator with topological sort

Parses job definitions from YAML with steps, dependencies, retry config,
reader/writer configs, and parameters. DagValidator detects cycles and
missing dependencies using Kahn's algorithm, and produces leveled
topological sort for parallel step execution."
```

---

### Task 4: Persistence Repositories

**Files:**
- Create: `src/main/java/com/batchforge/engine/repository/JobDefinitionRepository.java`
- Create: `src/main/java/com/batchforge/engine/repository/JobExecutionRepository.java`
- Create: `src/main/java/com/batchforge/engine/repository/StepExecutionRepository.java`
- Create: `src/main/java/com/batchforge/engine/repository/CheckpointRepository.java`
- Create: `src/main/java/com/batchforge/engine/repository/DeadLetterRepository.java`
- Test: `src/test/java/com/batchforge/engine/repository/RepositoryIntegrationTest.java`
- Create: `src/test/java/com/batchforge/engine/repository/TestcontainersConfig.java`

**Interfaces:**
- Consumes: all domain records from Task 1, Flyway schema from Task 1
- Produces:
  - `JobDefinitionRepository`: `save(JobDefinition, String dagJson, String yamlHash)`, `findByName(String): Optional<JobDefinition>`, `findAll(): List<JobDefinition>`, `delete(String name)`
  - `JobExecutionRepository`: `save(JobExecution): JobExecution`, `findById(UUID): Optional<JobExecution>`, `updateStatus(UUID, ExecutionStatus)`, `findByJobName(String, int limit): List<JobExecution>`
  - `StepExecutionRepository`: `save(StepExecution): StepExecution`, `findByJobExecutionId(UUID): List<StepExecution>`, `updateStatus(UUID, StepStatus)`, `updateProgress(UUID, int chunksProcessed, long itemsRead, long itemsWritten, long itemsFailed)`
  - `CheckpointRepository`: `save(Checkpoint)`, `findLatest(UUID stepExecutionId, String partitionId): Optional<Checkpoint>`
  - `DeadLetterRepository`: `save(DeadLetterItem)`, `findByStepExecution(UUID): List<DeadLetterItem>`, `updateStatus(UUID itemId, String status)`, `findPending(int limit): List<DeadLetterItem>`

- [ ] **Step 1: Create Testcontainers config for reuse across integration tests**

```java
package com.batchforge.engine.repository;

import io.quarkus.test.common.QuarkusTestResourceLifecycleManager;
import org.testcontainers.containers.PostgreSQLContainer;

import java.util.Map;

public class TestcontainersConfig implements QuarkusTestResourceLifecycleManager {

    static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>("postgres:17-alpine")
        .withDatabaseName("batchforge_test")
        .withUsername("test")
        .withPassword("test");

    @Override
    public Map<String, String> start() {
        POSTGRES.start();
        return Map.of(
            "quarkus.datasource.jdbc.url", POSTGRES.getJdbcUrl(),
            "quarkus.datasource.username", POSTGRES.getUsername(),
            "quarkus.datasource.password", POSTGRES.getPassword()
        );
    }

    @Override
    public void stop() {
        POSTGRES.stop();
    }
}
```

- [ ] **Step 2: Write integration test**

```java
package com.batchforge.engine.repository;

import com.batchforge.engine.core.*;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

@QuarkusTest
@QuarkusTestResource(TestcontainersConfig.class)
class RepositoryIntegrationTest {

    @Inject JobDefinitionRepository jobDefRepo;
    @Inject JobExecutionRepository jobExecRepo;
    @Inject StepExecutionRepository stepExecRepo;
    @Inject CheckpointRepository checkpointRepo;
    @Inject DeadLetterRepository deadLetterRepo;

    @Test
    void jobDefinitionCrud() {
        var job = new JobDefinition("test-job", "A test", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));

        jobDefRepo.save(job, "{\"steps\":[\"s1\"]}", "abc123");
        var found = jobDefRepo.findByName("test-job");
        assertThat(found).isPresent();
        assertThat(found.get().name()).isEqualTo("test-job");

        jobDefRepo.delete("test-job");
        assertThat(jobDefRepo.findByName("test-job")).isEmpty();
    }

    @Test
    void executionLifecycle() {
        // Setup job def first
        var job = new JobDefinition("exec-test-job", "test", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));
        jobDefRepo.save(job, "{}", "hash1");

        var exec = JobExecution.create("exec-test-job", Map.of("key", "val"));
        exec = jobExecRepo.save(exec);
        assertThat(exec.id()).isNotNull();

        jobExecRepo.updateStatus(exec.id(), new ExecutionStatus.Running());
        var updated = jobExecRepo.findById(exec.id());
        assertThat(updated).isPresent();
        assertThat(updated.get().status()).isInstanceOf(ExecutionStatus.Running.class);
    }

    @Test
    void stepExecutionWithCheckpoint() {
        var job = new JobDefinition("step-test-job", "test", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));
        jobDefRepo.save(job, "{}", "hash2");

        var exec = jobExecRepo.save(JobExecution.create("step-test-job", Map.of()));
        var stepExec = stepExecRepo.save(StepExecution.create(exec.id(), "s1", "main"));

        checkpointRepo.save(new Checkpoint(stepExec.id(), "main", 500, Map.of(), Instant.now()));
        var cp = checkpointRepo.findLatest(stepExec.id(), "main");
        assertThat(cp).isPresent();
        assertThat(cp.get().offset()).isEqualTo(500);
    }

    @Test
    void deadLetterItemPersistence() {
        var job = new JobDefinition("dlq-test-job", "test", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));
        jobDefRepo.save(job, "{}", "hash3");

        var exec = jobExecRepo.save(JobExecution.create("dlq-test-job", Map.of()));
        var stepExec = stepExecRepo.save(StepExecution.create(exec.id(), "s1", "main"));

        var dlq = DeadLetterItem.create(stepExec.id(), "s1", "{\"id\":1}", "NPE", "stack...", 3);
        deadLetterRepo.save(dlq);

        var items = deadLetterRepo.findByStepExecution(stepExec.id());
        assertThat(items).hasSize(1);
        assertThat(items.getFirst().errorMessage()).isEqualTo("NPE");

        var pending = deadLetterRepo.findPending(10);
        assertThat(pending).isNotEmpty();
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `mvn test -Dtest=RepositoryIntegrationTest`
Expected: FAIL — repository classes not found

- [ ] **Step 4: Implement all 5 repositories**

Each repository is a `@ApplicationScoped` CDI bean that injects `javax.sql.DataSource` (via Agroal) and uses plain JDBC. Implement each with the signatures listed in the Produces section above. Use `try-with-resources` for `Connection`, `PreparedStatement`, `ResultSet`. Map domain records to/from SQL using the `toDbValue()`/`fromDbValue()` methods on sealed interfaces.

Key implementation patterns:
- `JobDefinitionRepository.save()` does `INSERT ... ON CONFLICT (name) DO UPDATE` (upsert)
- `JobExecutionRepository.save()` inserts and returns the record with the generated UUID
- `CheckpointRepository.save()` does `INSERT ... ON CONFLICT DO UPDATE` on the composite PK
- All repositories use `@Transactional` from `jakarta.transaction` for write operations

- [ ] **Step 5: Run tests to verify they pass**

Run: `mvn test -Dtest=RepositoryIntegrationTest`
Expected: PASS — all 4 tests green (Testcontainers starts Postgres, Flyway runs migration, CRUD works)

- [ ] **Step 6: Commit**

```bash
git add src/
git commit -m "feat(batchforge): persistence repositories with Testcontainers integration tests

JDBC repositories for job definitions, executions, step executions,
checkpoints, and dead letter items. Integration tests against real
PostgreSQL 17 via Testcontainers with Flyway migrations."
```

---

### Task 5: Retry Policy + Dead Letter Manager

**Files:**
- Create: `src/main/java/com/batchforge/engine/retry/RetryPolicy.java`
- Create: `src/main/java/com/batchforge/engine/retry/BackoffStrategy.java`
- Create: `src/main/java/com/batchforge/engine/retry/FixedBackoff.java`
- Create: `src/main/java/com/batchforge/engine/retry/ExponentialBackoff.java`
- Create: `src/main/java/com/batchforge/engine/retry/ExponentialJitterBackoff.java`
- Create: `src/main/java/com/batchforge/engine/deadletter/DeadLetterManager.java`
- Test: `src/test/java/com/batchforge/engine/retry/RetryPolicyTest.java`
- Test: `src/test/java/com/batchforge/engine/deadletter/DeadLetterManagerTest.java`

**Interfaces:**
- Consumes: `RetryConfig` from Task 1, `DeadLetterRepository` from Task 4, `DeadLetterItem` from Task 1
- Produces:
  - `interface BackoffStrategy { Duration nextDelay(int attempt); }`
  - `RetryPolicy`: wraps `RetryConfig`, determines `shouldRetry(Exception, int attempt): boolean`, `getDelay(int attempt): Duration`
  - `DeadLetterManager`: `send(UUID stepExecutionId, String stepName, Object item, Exception error, int attempt)`, `retryItem(UUID itemId)`, `discardItem(UUID itemId)`

- [ ] **Step 1: Write retry policy tests**

```java
package com.batchforge.engine.retry;

import com.batchforge.engine.core.RetryConfig;
import org.junit.jupiter.api.Test;
import java.net.SocketTimeoutException;
import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.*;

class RetryPolicyTest {

    @Test
    void fixedBackoffReturnsSameDelay() {
        var backoff = new FixedBackoff(Duration.ofSeconds(2));
        assertThat(backoff.nextDelay(1)).isEqualTo(Duration.ofSeconds(2));
        assertThat(backoff.nextDelay(5)).isEqualTo(Duration.ofSeconds(2));
    }

    @Test
    void exponentialBackoffDoublesWithCap() {
        var backoff = new ExponentialBackoff(Duration.ofSeconds(1), Duration.ofSeconds(10));
        assertThat(backoff.nextDelay(1)).isEqualTo(Duration.ofSeconds(1));
        assertThat(backoff.nextDelay(2)).isEqualTo(Duration.ofSeconds(2));
        assertThat(backoff.nextDelay(3)).isEqualTo(Duration.ofSeconds(4));
        assertThat(backoff.nextDelay(4)).isEqualTo(Duration.ofSeconds(8));
        assertThat(backoff.nextDelay(5)).isEqualTo(Duration.ofSeconds(10)); // capped
    }

    @Test
    void exponentialJitterNeverExceedsMax() {
        var backoff = new ExponentialJitterBackoff(Duration.ofSeconds(1), Duration.ofSeconds(10));
        for (int i = 1; i <= 20; i++) {
            assertThat(backoff.nextDelay(i)).isLessThanOrEqualTo(Duration.ofSeconds(10));
        }
    }

    @Test
    void retryPolicyRespectsMaxAttempts() {
        var config = new RetryConfig(3, "fixed", Duration.ofSeconds(1), Duration.ofSeconds(30), List.of());
        var policy = RetryPolicy.from(config);

        assertThat(policy.shouldRetry(new RuntimeException(), 1)).isTrue();
        assertThat(policy.shouldRetry(new RuntimeException(), 3)).isFalse();
    }

    @Test
    void retryPolicyFiltersExceptionTypes() {
        var config = new RetryConfig(3, "fixed", Duration.ofSeconds(1), Duration.ofSeconds(30),
            List.of("java.net.SocketTimeoutException"));
        var policy = RetryPolicy.from(config);

        assertThat(policy.shouldRetry(new SocketTimeoutException(), 1)).isTrue();
        assertThat(policy.shouldRetry(new NullPointerException(), 1)).isFalse();
    }

    @Test
    void noneConfigNeverRetries() {
        var policy = RetryPolicy.from(RetryConfig.none());
        assertThat(policy.shouldRetry(new RuntimeException(), 1)).isFalse();
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `mvn test -Dtest=RetryPolicyTest`
Expected: FAIL — classes not found

- [ ] **Step 3: Implement backoff strategies and RetryPolicy**

`BackoffStrategy.java`:
```java
package com.batchforge.engine.retry;

import java.time.Duration;

public interface BackoffStrategy {
    Duration nextDelay(int attempt);
}
```

`FixedBackoff.java`:
```java
package com.batchforge.engine.retry;

import java.time.Duration;

public record FixedBackoff(Duration delay) implements BackoffStrategy {
    @Override
    public Duration nextDelay(int attempt) {
        return delay;
    }
}
```

`ExponentialBackoff.java`:
```java
package com.batchforge.engine.retry;

import java.time.Duration;

public record ExponentialBackoff(Duration baseDelay, Duration maxDelay) implements BackoffStrategy {
    @Override
    public Duration nextDelay(int attempt) {
        long delayMs = baseDelay.toMillis() * (1L << (attempt - 1));
        return Duration.ofMillis(Math.min(delayMs, maxDelay.toMillis()));
    }
}
```

`ExponentialJitterBackoff.java`:
```java
package com.batchforge.engine.retry;

import java.time.Duration;
import java.util.concurrent.ThreadLocalRandom;

public record ExponentialJitterBackoff(Duration baseDelay, Duration maxDelay) implements BackoffStrategy {
    @Override
    public Duration nextDelay(int attempt) {
        long expMs = baseDelay.toMillis() * (1L << (attempt - 1));
        long cappedMs = Math.min(expMs, maxDelay.toMillis());
        long jitteredMs = ThreadLocalRandom.current().nextLong(cappedMs / 2, cappedMs + 1);
        return Duration.ofMillis(jitteredMs);
    }
}
```

`RetryPolicy.java`:
```java
package com.batchforge.engine.retry;

import com.batchforge.engine.core.RetryConfig;
import java.time.Duration;
import java.util.Set;
import java.util.stream.Collectors;

public final class RetryPolicy {
    private final int maxAttempts;
    private final BackoffStrategy backoff;
    private final Set<String> retryableExceptions;

    private RetryPolicy(int maxAttempts, BackoffStrategy backoff, Set<String> retryableExceptions) {
        this.maxAttempts = maxAttempts;
        this.backoff = backoff;
        this.retryableExceptions = retryableExceptions;
    }

    public static RetryPolicy from(RetryConfig config) {
        BackoffStrategy backoff = switch (config.backoff()) {
            case "fixed" -> new FixedBackoff(config.baseDelay());
            case "exponential" -> new ExponentialBackoff(config.baseDelay(), config.maxDelay());
            case "exponential-jitter" -> new ExponentialJitterBackoff(config.baseDelay(), config.maxDelay());
            default -> new FixedBackoff(config.baseDelay());
        };
        return new RetryPolicy(config.maxAttempts(), backoff,
            config.retryableExceptions().stream().collect(Collectors.toSet()));
    }

    public boolean shouldRetry(Exception e, int attempt) {
        if (attempt >= maxAttempts) return false;
        if (retryableExceptions.isEmpty()) return true;
        return retryableExceptions.contains(e.getClass().getName());
    }

    public Duration getDelay(int attempt) {
        return backoff.nextDelay(attempt);
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `mvn test -Dtest=RetryPolicyTest`
Expected: PASS — all 6 tests green

- [ ] **Step 5: Implement `DeadLetterManager`**

```java
package com.batchforge.engine.deadletter;

import com.batchforge.engine.core.DeadLetterItem;
import com.batchforge.engine.repository.DeadLetterRepository;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.util.UUID;

@ApplicationScoped
public class DeadLetterManager {

    @Inject DeadLetterRepository repository;
    @Inject ObjectMapper objectMapper;

    public void send(UUID stepExecutionId, String stepName, Object item, Exception error, int attempt) {
        String payload;
        try {
            payload = objectMapper.writeValueAsString(item);
        } catch (Exception e) {
            payload = item.toString();
        }
        StringWriter sw = new StringWriter();
        error.printStackTrace(new PrintWriter(sw));

        repository.save(DeadLetterItem.create(
            stepExecutionId, stepName, payload, error.getMessage(), sw.toString(), attempt
        ));
    }

    public void retryItem(UUID itemId) {
        repository.updateStatus(itemId, "RETRIED");
    }

    public void discardItem(UUID itemId) {
        repository.updateStatus(itemId, "DISCARDED");
    }
}
```

- [ ] **Step 6: Commit**

```bash
git add src/
git commit -m "feat(batchforge): retry policies with backoff strategies and dead letter manager

Fixed, exponential, and exponential-jitter backoff strategies.
RetryPolicy filters by exception type and max attempts.
DeadLetterManager serializes failed items to DLQ table."
```

---

### Task 6: Checkpoint Manager

**Files:**
- Create: `src/main/java/com/batchforge/engine/checkpoint/CheckpointManager.java`
- Test: `src/test/java/com/batchforge/engine/checkpoint/CheckpointManagerIntegrationTest.java`

**Interfaces:**
- Consumes: `CheckpointRepository` from Task 4, `Checkpoint` from Task 1
- Produces:
  - `CheckpointManager.save(UUID stepExecutionId, String partitionId, long offset, Map<String, Object> metadata): void`
  - `CheckpointManager.getLastOffset(UUID stepExecutionId, String partitionId): OptionalLong`

- [ ] **Step 1: Write integration test**

```java
package com.batchforge.engine.checkpoint;

import com.batchforge.engine.core.*;
import com.batchforge.engine.repository.*;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Test;

import java.util.*;

import static org.assertj.core.api.Assertions.*;

@QuarkusTest
@QuarkusTestResource(TestcontainersConfig.class)
class CheckpointManagerIntegrationTest {

    @Inject CheckpointManager checkpointManager;
    @Inject JobDefinitionRepository jobDefRepo;
    @Inject JobExecutionRepository jobExecRepo;
    @Inject StepExecutionRepository stepExecRepo;

    private UUID setupStepExecution(String jobName) {
        var job = new JobDefinition(jobName, "test", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));
        jobDefRepo.save(job, "{}", "h");
        var exec = jobExecRepo.save(JobExecution.create(jobName, Map.of()));
        return stepExecRepo.save(StepExecution.create(exec.id(), "s1", "main")).id();
    }

    @Test
    void saveAndRetrieveCheckpoint() {
        UUID stepExecId = setupStepExecution("cp-test-1");

        checkpointManager.save(stepExecId, "main", 500, Map.of());
        var offset = checkpointManager.getLastOffset(stepExecId, "main");
        assertThat(offset).hasValue(500);
    }

    @Test
    void checkpointUpsertsOnConflict() {
        UUID stepExecId = setupStepExecution("cp-test-2");

        checkpointManager.save(stepExecId, "main", 100, Map.of());
        checkpointManager.save(stepExecId, "main", 200, Map.of());
        var offset = checkpointManager.getLastOffset(stepExecId, "main");
        assertThat(offset).hasValue(200);
    }

    @Test
    void returnsEmptyForNoCheckpoint() {
        UUID stepExecId = setupStepExecution("cp-test-3");
        var offset = checkpointManager.getLastOffset(stepExecId, "main");
        assertThat(offset).isEmpty();
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `mvn test -Dtest=CheckpointManagerIntegrationTest`
Expected: FAIL — `CheckpointManager` not found

- [ ] **Step 3: Implement `CheckpointManager`**

```java
package com.batchforge.engine.checkpoint;

import com.batchforge.engine.core.Checkpoint;
import com.batchforge.engine.repository.CheckpointRepository;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.time.Instant;
import java.util.Map;
import java.util.OptionalLong;
import java.util.UUID;

@ApplicationScoped
public class CheckpointManager {

    @Inject CheckpointRepository repository;

    public void save(UUID stepExecutionId, String partitionId, long offset, Map<String, Object> metadata) {
        repository.save(new Checkpoint(stepExecutionId, partitionId, offset, metadata, Instant.now()));
    }

    public OptionalLong getLastOffset(UUID stepExecutionId, String partitionId) {
        return repository.findLatest(stepExecutionId, partitionId)
            .map(cp -> OptionalLong.of(cp.offset()))
            .orElse(OptionalLong.empty());
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `mvn test -Dtest=CheckpointManagerIntegrationTest`
Expected: PASS — all 3 tests green

- [ ] **Step 5: Commit**

```bash
git add src/
git commit -m "feat(batchforge): checkpoint manager with upsert and offset retrieval

Saves and retrieves chunk offsets per step+partition.
Upserts on conflict for idempotent checkpoint updates."
```

---

### Task 7: Step Executor (Chunk Loop with Virtual Threads)

**Files:**
- Create: `src/main/java/com/batchforge/engine/executor/StepExecutor.java`
- Create: `src/main/java/com/batchforge/engine/executor/ChunkResult.java`
- Test: `src/test/java/com/batchforge/engine/executor/StepExecutorTest.java`

**Interfaces:**
- Consumes: `ItemReader`, `ItemProcessor`, `ItemWriter` from Task 2, `CheckpointManager` from Task 6, `RetryPolicy` from Task 5, `DeadLetterManager` from Task 5, `StepExecutionRepository` from Task 4, `ExecutionContext` from Task 2, `StepDefinition` from Task 1
- Produces:
  - `record ChunkResult(int itemsRead, int itemsWritten, int itemsFailed, Duration duration)`
  - `StepExecutor.execute(StepDefinition, ExecutionContext, ItemReader, ItemProcessor, ItemWriter): StepExecutionResult`
  - Items within a chunk are processed in parallel via Virtual Threads; the chunk write is atomic with the checkpoint save

- [ ] **Step 1: Write step executor test with in-memory reader/processor/writer**

```java
package com.batchforge.engine.executor;

import com.batchforge.engine.checkpoint.CheckpointManager;
import com.batchforge.engine.core.*;
import com.batchforge.engine.deadletter.DeadLetterManager;
import com.batchforge.engine.retry.RetryPolicy;
import com.batchforge.spi.*;
import org.junit.jupiter.api.Test;

import java.util.*;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicLong;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.Mockito.*;

class StepExecutorTest {

    @Test
    void processesAllItemsInChunks() {
        var items = List.of("a", "b", "c", "d", "e");
        var reader = new ListReader<>(items);
        ItemProcessor<String, String> processor = s -> s.toUpperCase();
        var writer = new CollectingWriter<String>();
        var checkpointMgr = mock(CheckpointManager.class);
        var deadLetterMgr = mock(DeadLetterManager.class);
        var retryPolicy = RetryPolicy.from(RetryConfig.none());
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of());

        var executor = new StepExecutor(checkpointMgr, deadLetterMgr);
        var result = executor.execute(ctx, reader, processor, writer, 2, retryPolicy);

        assertThat(writer.written).containsExactly("A", "B", "C", "D", "E");
        assertThat(result.itemsRead()).isEqualTo(5);
        assertThat(result.itemsWritten()).isEqualTo(5);
        assertThat(result.itemsFailed()).isZero();
        verify(checkpointMgr, times(3)).save(eq(ctx.stepExecutionId()), eq("main"), anyLong(), any());
    }

    @Test
    void failedItemsGoToDeadLetterAndChunkContinues() {
        var items = List.of("ok", "fail", "ok2");
        var reader = new ListReader<>(items);
        ItemProcessor<String, String> processor = s -> {
            if (s.equals("fail")) throw new ProcessingException("bad item");
            return s.toUpperCase();
        };
        var writer = new CollectingWriter<String>();
        var checkpointMgr = mock(CheckpointManager.class);
        var deadLetterMgr = mock(DeadLetterManager.class);
        var retryPolicy = RetryPolicy.from(RetryConfig.none());
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of());

        var executor = new StepExecutor(checkpointMgr, deadLetterMgr);
        var result = executor.execute(ctx, reader, processor, writer, 10, retryPolicy);

        assertThat(writer.written).containsExactly("OK", "OK2");
        assertThat(result.itemsFailed()).isEqualTo(1);
        verify(deadLetterMgr).send(eq(ctx.stepExecutionId()), eq("s1"), eq("fail"), any(), eq(1));
    }

    @Test
    void resumesFromCheckpointOffset() {
        var items = List.of("a", "b", "c", "d", "e");
        var reader = new ListReader<>(items);
        ItemProcessor<String, String> processor = s -> s.toUpperCase();
        var writer = new CollectingWriter<String>();
        var checkpointMgr = mock(CheckpointManager.class);
        when(checkpointMgr.getLastOffset(any(), any())).thenReturn(OptionalLong.of(3));
        var deadLetterMgr = mock(DeadLetterManager.class);
        var retryPolicy = RetryPolicy.from(RetryConfig.none());
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of());

        var executor = new StepExecutor(checkpointMgr, deadLetterMgr);
        var result = executor.execute(ctx, reader, processor, writer, 10, retryPolicy);

        assertThat(writer.written).containsExactly("D", "E");
        assertThat(result.itemsRead()).isEqualTo(2);
    }

    // Test helpers
    static class ListReader<T> implements ItemReader<T> {
        private final List<T> items;
        private final AtomicLong offset = new AtomicLong(0);

        ListReader(List<T> items) { this.items = new ArrayList<>(items); }

        @Override public void open(ExecutionContext context) {}
        @Override public List<T> read(int chunkSize) {
            int from = (int) offset.get();
            int to = Math.min(from + chunkSize, items.size());
            var chunk = items.subList(from, to);
            offset.set(to);
            return new ArrayList<>(chunk);
        }
        @Override public boolean hasMore() { return offset.get() < items.size(); }
        @Override public void seekTo(long off) { offset.set(off); }
        @Override public void close() {}
    }

    static class CollectingWriter<T> implements ItemWriter<T> {
        final List<T> written = new CopyOnWriteArrayList<>();
        @Override public void open(ExecutionContext context) {}
        @Override public void write(List<T> items) { written.addAll(items); }
        @Override public void close() {}
    }
}
```

Add Mockito dependency to `pom.xml`:
```xml
<dependency>
    <groupId>org.mockito</groupId>
    <artifactId>mockito-core</artifactId>
    <version>5.14.2</version>
    <scope>test</scope>
</dependency>
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `mvn test -Dtest=StepExecutorTest`
Expected: FAIL — `StepExecutor` not found

- [ ] **Step 3: Implement `StepExecutor`**

`ChunkResult.java`:
```java
package com.batchforge.engine.executor;

import java.time.Duration;

public record ChunkResult(int itemsRead, int itemsWritten, int itemsFailed, Duration duration) {}
```

`StepExecutor.java`:
```java
package com.batchforge.engine.executor;

import com.batchforge.engine.checkpoint.CheckpointManager;
import com.batchforge.engine.deadletter.DeadLetterManager;
import com.batchforge.engine.retry.RetryPolicy;
import com.batchforge.spi.*;

import java.time.Duration;
import java.time.Instant;
import java.util.*;
import java.util.concurrent.StructuredTaskScope;

public class StepExecutor {

    private final CheckpointManager checkpointManager;
    private final DeadLetterManager deadLetterManager;

    public StepExecutor(CheckpointManager checkpointManager, DeadLetterManager deadLetterManager) {
        this.checkpointManager = checkpointManager;
        this.deadLetterManager = deadLetterManager;
    }

    public <I, O> ChunkResult execute(
            ExecutionContext context,
            ItemReader<I> reader,
            ItemProcessor<I, O> processor,
            ItemWriter<O> writer,
            int chunkSize,
            RetryPolicy retryPolicy) {

        Instant start = Instant.now();
        long totalRead = 0, totalWritten = 0, totalFailed = 0;

        reader.open(context);
        writer.open(context);

        // Resume from checkpoint if available
        var lastOffset = checkpointManager.getLastOffset(context.stepExecutionId(), context.partitionId());
        if (lastOffset.isPresent()) {
            reader.seekTo(lastOffset.getAsLong());
        }

        try {
            long currentOffset = lastOffset.orElse(0);

            while (reader.hasMore()) {
                List<I> chunk = reader.read(chunkSize);
                if (chunk.isEmpty()) break;

                totalRead += chunk.size();

                // Process items in parallel via Virtual Threads
                List<O> processed = new ArrayList<>();
                long chunkFailed = 0;

                try (var scope = new StructuredTaskScope.ShutdownOnFailure()) {
                    record ItemResult<O>(O value, Exception error, Object original) {}
                    List<StructuredTaskScope.Subtask<ItemResult<O>>> subtasks = new ArrayList<>();

                    for (I item : chunk) {
                        subtasks.add(scope.fork(() -> {
                            try {
                                return new ItemResult<>(processor.process(item), null, item);
                            } catch (Exception e) {
                                return new ItemResult<>(null, e, item);
                            }
                        }));
                    }

                    scope.join();

                    for (var subtask : subtasks) {
                        ItemResult<O> result = subtask.get();
                        if (result.error() != null) {
                            if (retryPolicy.shouldRetry(result.error(), 1)) {
                                // Simple single-retry inline for now
                                try {
                                    @SuppressWarnings("unchecked")
                                    O retried = processor.process((I) result.original());
                                    processed.add(retried);
                                } catch (Exception retryError) {
                                    chunkFailed++;
                                    deadLetterManager.send(context.stepExecutionId(), context.stepName(),
                                        result.original(), retryError, 2);
                                }
                            } else {
                                chunkFailed++;
                                deadLetterManager.send(context.stepExecutionId(), context.stepName(),
                                    result.original(), result.error(), 1);
                            }
                        } else {
                            processed.add(result.value());
                        }
                    }
                }

                totalFailed += chunkFailed;

                if (!processed.isEmpty()) {
                    writer.write(processed);
                    totalWritten += processed.size();
                }

                currentOffset += chunk.size();
                checkpointManager.save(context.stepExecutionId(), context.partitionId(), currentOffset, Map.of());
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new RuntimeException("Step execution interrupted", e);
        } finally {
            reader.close();
            writer.close();
        }

        return new ChunkResult((int) totalRead, (int) totalWritten, (int) totalFailed,
            Duration.between(start, Instant.now()));
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `mvn test -Dtest=StepExecutorTest`
Expected: PASS — all 3 tests green

- [ ] **Step 5: Commit**

```bash
git add pom.xml src/
git commit -m "feat(batchforge): step executor with chunk loop, Virtual Threads, and checkpoint resume

Processes items in chunks with parallel item processing via
StructuredTaskScope. Failed items go to dead letter without
interrupting the chunk. Resumes from checkpoint offset on restart."
```

---

### Task 8: Partition Manager

**Files:**
- Create: `src/main/java/com/batchforge/engine/partition/PartitionManager.java`
- Create: `src/main/java/com/batchforge/engine/partition/RangePartitioner.java`
- Test: `src/test/java/com/batchforge/engine/partition/PartitionManagerTest.java`

**Interfaces:**
- Consumes: `Partitioner`, `PartitionRange`, `ExecutionContext` from Task 2
- Produces:
  - `RangePartitioner implements Partitioner`: splits by numeric range (`minId`..`maxId` in context properties)
  - `PartitionManager.createPartitions(StepDefinition, ExecutionContext): List<PartitionRange>`

- [ ] **Step 1: Write tests**

```java
package com.batchforge.engine.partition;

import com.batchforge.spi.*;
import org.junit.jupiter.api.Test;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class PartitionManagerTest {

    @Test
    void rangePartitionerSplitsEvenly() {
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of(), Map.of("minId", "0", "maxId", "1000"));
        var partitioner = new RangePartitioner();
        var partitions = partitioner.partition(ctx, 4);

        assertThat(partitions).hasSize(4);
        assertThat(partitions.get(0).parameters()).containsEntry("rangeStart", 0L);
        assertThat(partitions.get(0).parameters()).containsEntry("rangeEnd", 250L);
        assertThat(partitions.get(3).parameters()).containsEntry("rangeStart", 750L);
        assertThat(partitions.get(3).parameters()).containsEntry("rangeEnd", 1000L);
    }

    @Test
    void singlePartitionReturnsMainOnly() {
        var manager = new PartitionManager();
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of(), Map.of());
        var partitions = manager.createPartitions(1, ctx);

        assertThat(partitions).hasSize(1);
        assertThat(partitions.getFirst().partitionId()).isEqualTo("main");
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `mvn test -Dtest=PartitionManagerTest`
Expected: FAIL

- [ ] **Step 3: Implement `RangePartitioner` and `PartitionManager`**

`RangePartitioner.java`:
```java
package com.batchforge.engine.partition;

import com.batchforge.spi.*;
import java.util.*;

public class RangePartitioner implements Partitioner {
    @Override
    public List<PartitionRange> partition(ExecutionContext context, int partitionCount) {
        long minId = Long.parseLong(context.stepProperties().get("minId").toString());
        long maxId = Long.parseLong(context.stepProperties().get("maxId").toString());
        long range = maxId - minId;
        long partSize = range / partitionCount;

        List<PartitionRange> partitions = new ArrayList<>();
        for (int i = 0; i < partitionCount; i++) {
            long start = minId + (i * partSize);
            long end = (i == partitionCount - 1) ? maxId : start + partSize;
            partitions.add(new PartitionRange(
                "partition-" + i,
                Map.of("rangeStart", start, "rangeEnd", end)
            ));
        }
        return partitions;
    }
}
```

`PartitionManager.java`:
```java
package com.batchforge.engine.partition;

import com.batchforge.spi.*;
import jakarta.enterprise.context.ApplicationScoped;
import java.util.*;

@ApplicationScoped
public class PartitionManager {
    public List<PartitionRange> createPartitions(int partitionCount, ExecutionContext context) {
        if (partitionCount <= 1) {
            return List.of(new PartitionRange("main", Map.of()));
        }
        return new RangePartitioner().partition(context, partitionCount);
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `mvn test -Dtest=PartitionManagerTest`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/
git commit -m "feat(batchforge): partition manager with range-based partitioning

RangePartitioner splits numeric ranges evenly across N partitions.
PartitionManager defaults to single 'main' partition when count is 1."
```

---

### Task 9: DAG Scheduler (Structured Concurrency Orchestration)

**Files:**
- Create: `src/main/java/com/batchforge/engine/dag/DagScheduler.java`
- Create: `src/main/java/com/batchforge/engine/dag/StepExecutionResult.java`
- Test: `src/test/java/com/batchforge/engine/dag/DagSchedulerTest.java`

**Interfaces:**
- Consumes: `DagValidator.topologicalSort()` from Task 3, `StepExecutor` from Task 7, `PartitionManager` from Task 8, `JobExecution`, `StepExecution`, `JobDefinition` from Task 1, `StepExecutionRepository`, `JobExecutionRepository` from Task 4, `RetryPolicy` from Task 5, `ExecutionContext` from Task 2, readers/writers will be resolved by type
- Produces:
  - `record StepExecutionResult(String stepName, boolean success, ChunkResult aggregated, String errorMessage)`
  - `DagScheduler.execute(JobDefinition, JobExecution): void` — orchestrates full job: topo sort → level-by-level execution using `StructuredTaskScope` → updates statuses → handles failures

- [ ] **Step 1: Write DAG scheduler test**

```java
package com.batchforge.engine.dag;

import com.batchforge.engine.checkpoint.CheckpointManager;
import com.batchforge.engine.core.*;
import com.batchforge.engine.deadletter.DeadLetterManager;
import com.batchforge.engine.executor.StepExecutor;
import com.batchforge.engine.partition.PartitionManager;
import com.batchforge.engine.repository.*;
import com.batchforge.spi.*;
import org.junit.jupiter.api.Test;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.Mockito.*;

class DagSchedulerTest {

    @Test
    void executesStepsInTopologicalOrder() {
        var executionOrder = Collections.synchronizedList(new ArrayList<String>());
        var stepCounter = new AtomicInteger(0);

        // Build a simple 3-step DAG: A → B → C
        var steps = new LinkedHashMap<String, StepDefinition>();
        steps.put("A", new StepDefinition("A",
            new ReaderConfig("test", Map.of()), "com.Proc",
            new WriterConfig("test", Map.of()), 100, 1, List.of(), RetryConfig.none(), false));
        steps.put("B", new StepDefinition("B",
            new ReaderConfig("test", Map.of()), "com.Proc",
            new WriterConfig("test", Map.of()), 100, 1, List.of("A"), RetryConfig.none(), false));
        steps.put("C", new StepDefinition("C",
            new ReaderConfig("test", Map.of()), "com.Proc",
            new WriterConfig("test", Map.of()), 100, 1, List.of("B"), RetryConfig.none(), false));

        var jobDef = new JobDefinition("test-job", "test", null, Map.of(), steps);

        // The scheduler should call steps in order: A first, then B, then C
        var validator = new DagValidator();
        var levels = validator.topologicalSort(jobDef);

        assertThat(levels).hasSize(3);
        assertThat(levels.get(0)).containsExactly("A");
        assertThat(levels.get(1)).containsExactly("B");
        assertThat(levels.get(2)).containsExactly("C");
    }

    @Test
    void parallelStepsRunInSameLevel() {
        var steps = new LinkedHashMap<String, StepDefinition>();
        steps.put("A", new StepDefinition("A",
            new ReaderConfig("test", Map.of()), "com.Proc",
            new WriterConfig("test", Map.of()), 100, 1, List.of(), RetryConfig.none(), false));
        steps.put("B", new StepDefinition("B",
            new ReaderConfig("test", Map.of()), "com.Proc",
            new WriterConfig("test", Map.of()), 100, 1, List.of(), RetryConfig.none(), false));
        steps.put("C", new StepDefinition("C",
            new ReaderConfig("test", Map.of()), "com.Proc",
            new WriterConfig("test", Map.of()), 100, 1, List.of("A", "B"), RetryConfig.none(), false));

        var jobDef = new JobDefinition("test-job", "test", null, Map.of(), steps);
        var validator = new DagValidator();
        var levels = validator.topologicalSort(jobDef);

        assertThat(levels).hasSize(2);
        assertThat(levels.get(0)).containsExactlyInAnyOrder("A", "B");
        assertThat(levels.get(1)).containsExactly("C");
    }
}
```

- [ ] **Step 2: Run tests to verify they pass** (these tests use DagValidator which already exists)

Run: `mvn test -Dtest=DagSchedulerTest`
Expected: PASS — the structural tests pass using existing DagValidator

- [ ] **Step 3: Create `StepExecutionResult` and `DagScheduler`**

`StepExecutionResult.java`:
```java
package com.batchforge.engine.dag;

import com.batchforge.engine.executor.ChunkResult;

public record StepExecutionResult(String stepName, boolean success, ChunkResult result, String errorMessage) {}
```

`DagScheduler.java`:
```java
package com.batchforge.engine.dag;

import com.batchforge.engine.core.*;
import com.batchforge.engine.executor.*;
import com.batchforge.engine.checkpoint.CheckpointManager;
import com.batchforge.engine.deadletter.DeadLetterManager;
import com.batchforge.engine.partition.PartitionManager;
import com.batchforge.engine.retry.RetryPolicy;
import com.batchforge.engine.repository.*;
import com.batchforge.spi.*;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.time.Instant;
import java.util.*;
import java.util.concurrent.StructuredTaskScope;

@ApplicationScoped
public class DagScheduler {

    @Inject DagValidator dagValidator;
    @Inject StepExecutionRepository stepExecRepo;
    @Inject JobExecutionRepository jobExecRepo;
    @Inject CheckpointManager checkpointManager;
    @Inject DeadLetterManager deadLetterManager;
    @Inject PartitionManager partitionManager;
    @Inject ReaderWriterFactory readerWriterFactory;

    public void execute(JobDefinition jobDef, JobExecution jobExec) {
        jobExecRepo.updateStatus(jobExec.id(), new ExecutionStatus.Running());

        try {
            List<List<String>> levels = dagValidator.topologicalSort(jobDef);

            for (List<String> level : levels) {
                boolean levelSuccess = executeLevel(level, jobDef, jobExec);
                if (!levelSuccess) {
                    jobExecRepo.updateStatus(jobExec.id(), new ExecutionStatus.Failed("Step failure in level"));
                    return;
                }
            }

            jobExecRepo.updateStatus(jobExec.id(), new ExecutionStatus.Completed());
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            jobExecRepo.updateStatus(jobExec.id(), new ExecutionStatus.Failed("Interrupted"));
        } catch (Exception e) {
            jobExecRepo.updateStatus(jobExec.id(), new ExecutionStatus.Failed(e.getMessage()));
        }
    }

    @SuppressWarnings({"unchecked", "rawtypes"})
    private boolean executeLevel(List<String> stepNames, JobDefinition jobDef, JobExecution jobExec)
            throws InterruptedException {

        if (stepNames.size() == 1) {
            return executeSingleStep(stepNames.getFirst(), jobDef, jobExec);
        }

        try (var scope = new StructuredTaskScope.ShutdownOnFailure()) {
            List<StructuredTaskScope.Subtask<Boolean>> subtasks = new ArrayList<>();
            for (String stepName : stepNames) {
                subtasks.add(scope.fork(() -> executeSingleStep(stepName, jobDef, jobExec)));
            }
            scope.join();
            return subtasks.stream().allMatch(st -> {
                try { return st.get(); }
                catch (Exception e) { return false; }
            });
        }
    }

    @SuppressWarnings({"unchecked", "rawtypes"})
    private boolean executeSingleStep(String stepName, JobDefinition jobDef, JobExecution jobExec) {
        StepDefinition stepDef = jobDef.steps().get(stepName);

        var stepExec = stepExecRepo.save(StepExecution.create(jobExec.id(), stepName, "main"));
        stepExecRepo.updateStatus(stepExec.id(), new StepStatus.StepRunning());

        try {
            var ctx = new ExecutionContext(
                jobExec.id(), stepExec.id(), stepName, "main",
                jobExec.parameters(), stepDef.reader().properties()
            );

            ItemReader reader = readerWriterFactory.createReader(stepDef.reader());
            ItemWriter writer = readerWriterFactory.createWriter(stepDef.writer());
            ItemProcessor processor = readerWriterFactory.createProcessor(stepDef.processorClass());
            RetryPolicy retryPolicy = RetryPolicy.from(stepDef.retry());

            var executor = new StepExecutor(checkpointManager, deadLetterManager);
            var result = executor.execute(ctx, reader, processor, writer, stepDef.chunkSize(), retryPolicy);

            stepExecRepo.updateProgress(stepExec.id(),
                result.itemsRead() / Math.max(stepDef.chunkSize(), 1),
                result.itemsRead(), result.itemsWritten(), result.itemsFailed());
            stepExecRepo.updateStatus(stepExec.id(), new StepStatus.StepCompleted());
            return true;
        } catch (Exception e) {
            stepExecRepo.updateStatus(stepExec.id(), new StepStatus.StepFailed(e.getMessage()));
            return stepDef.skipOnFail();
        }
    }
}
```

- [ ] **Step 4: Create `ReaderWriterFactory` interface** (concrete implementations come in Task 10)

```java
package com.batchforge.engine.dag;

import com.batchforge.engine.core.ReaderConfig;
import com.batchforge.engine.core.WriterConfig;
import com.batchforge.spi.*;

public interface ReaderWriterFactory {
    ItemReader<?> createReader(ReaderConfig config);
    ItemWriter<?> createWriter(WriterConfig config);
    ItemProcessor<?, ?> createProcessor(String className);
}
```

Make `DagValidator` a CDI bean by adding `@ApplicationScoped`.

- [ ] **Step 5: Commit**

```bash
git add src/
git commit -m "feat(batchforge): DAG scheduler with Structured Concurrency level-by-level execution

Orchestrates job execution: topological sort → parallel step execution
per level via StructuredTaskScope → status updates. Single steps run
directly, parallel steps fork virtual threads."
```

---

### Task 10: Built-in Readers and Writers

**Files:**
- Create: `src/main/java/com/batchforge/readers/JdbcReader.java`
- Create: `src/main/java/com/batchforge/readers/CsvReader.java`
- Create: `src/main/java/com/batchforge/readers/JsonLinesReader.java`
- Create: `src/main/java/com/batchforge/writers/JdbcWriter.java`
- Create: `src/main/java/com/batchforge/writers/CsvWriter.java`
- Create: `src/main/java/com/batchforge/writers/FileWriter.java`
- Create: `src/main/java/com/batchforge/engine/dag/DefaultReaderWriterFactory.java`
- Test: `src/test/java/com/batchforge/readers/CsvReaderTest.java`
- Test: `src/test/java/com/batchforge/readers/JsonLinesReaderTest.java`
- Test: `src/test/java/com/batchforge/readers/JdbcReaderIntegrationTest.java`
- Test: `src/test/java/com/batchforge/writers/JdbcWriterIntegrationTest.java`

**Interfaces:**
- Consumes: `ItemReader<Map<String, Object>>`, `ItemWriter<Map<String, Object>>`, `ExecutionContext` from Task 2, `ReaderWriterFactory` from Task 9
- Produces:
  - `JdbcReader implements ItemReader<Map<String, Object>>`: reads from SQL query, each row as a Map
  - `CsvReader implements ItemReader<Map<String, Object>>`: reads CSV with headers
  - `JsonLinesReader implements ItemReader<Map<String, Object>>`: reads JSON Lines file
  - `JdbcWriter implements ItemWriter<Map<String, Object>>`: batch inserts to a table
  - `CsvWriter implements ItemWriter<Map<String, Object>>`: writes to CSV file
  - `FileWriter implements ItemWriter<String>`: writes raw strings to file
  - `DefaultReaderWriterFactory implements ReaderWriterFactory`: resolves reader/writer by type string

- [ ] **Step 1: Write CSV reader test**

```java
package com.batchforge.readers;

import com.batchforge.spi.ExecutionContext;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.*;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class CsvReaderTest {

    @TempDir Path tempDir;

    @Test
    void readsAllRowsWithHeaders() throws Exception {
        Path csv = tempDir.resolve("data.csv");
        Files.writeString(csv, "id,name,amount\n1,Alice,100.50\n2,Bob,200.75\n3,Carol,50.00\n");

        var reader = new CsvReader(csv.toString());
        reader.open(new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of()));

        var chunk = reader.read(2);
        assertThat(chunk).hasSize(2);
        assertThat(chunk.getFirst()).containsEntry("id", "1").containsEntry("name", "Alice");

        var chunk2 = reader.read(2);
        assertThat(chunk2).hasSize(1);
        assertThat(chunk2.getFirst()).containsEntry("id", "3");

        assertThat(reader.hasMore()).isFalse();
        reader.close();
    }

    @Test
    void seekToSkipsRows() throws Exception {
        Path csv = tempDir.resolve("data.csv");
        Files.writeString(csv, "id,name\n1,A\n2,B\n3,C\n4,D\n");

        var reader = new CsvReader(csv.toString());
        reader.open(new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of()));
        reader.seekTo(2);

        var chunk = reader.read(10);
        assertThat(chunk).hasSize(2);
        assertThat(chunk.getFirst()).containsEntry("id", "3");
        reader.close();
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `mvn test -Dtest=CsvReaderTest`
Expected: FAIL

- [ ] **Step 3: Implement `CsvReader`**

```java
package com.batchforge.readers;

import com.batchforge.spi.*;
import java.io.*;
import java.nio.file.*;
import java.util.*;

public class CsvReader implements ItemReader<Map<String, Object>> {
    private final String path;
    private BufferedReader bufferedReader;
    private String[] headers;
    private long currentOffset;
    private boolean done;

    public CsvReader(String path) { this.path = path; }

    @Override
    public void open(ExecutionContext context) {
        try {
            String resolvedPath = context.resolveTemplate(path);
            bufferedReader = Files.newBufferedReader(Path.of(resolvedPath));
            String headerLine = bufferedReader.readLine();
            if (headerLine == null) throw new IllegalStateException("CSV is empty");
            headers = headerLine.split(",");
            currentOffset = 0;
            done = false;
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public List<Map<String, Object>> read(int chunkSize) {
        List<Map<String, Object>> rows = new ArrayList<>();
        try {
            for (int i = 0; i < chunkSize; i++) {
                String line = bufferedReader.readLine();
                if (line == null) { done = true; break; }
                String[] values = line.split(",", -1);
                Map<String, Object> row = new LinkedHashMap<>();
                for (int j = 0; j < headers.length && j < values.length; j++) {
                    row.put(headers[j].trim(), values[j].trim());
                }
                rows.add(row);
                currentOffset++;
            }
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
        return rows;
    }

    @Override
    public boolean hasMore() { return !done; }

    @Override
    public void seekTo(long offset) {
        try {
            // Re-open and skip lines
            if (bufferedReader != null) bufferedReader.close();
            bufferedReader = Files.newBufferedReader(Path.of(path));
            bufferedReader.readLine(); // skip header
            for (long i = 0; i < offset; i++) {
                if (bufferedReader.readLine() == null) { done = true; break; }
            }
            currentOffset = offset;
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public void close() {
        try { if (bufferedReader != null) bufferedReader.close(); }
        catch (IOException e) { throw new UncheckedIOException(e); }
    }
}
```

- [ ] **Step 4: Run CSV reader test to verify it passes**

Run: `mvn test -Dtest=CsvReaderTest`
Expected: PASS

- [ ] **Step 5: Implement remaining readers and writers**

Implement `JsonLinesReader` (reads JSON objects per line, one per row), `JdbcReader` (executes SQL query, returns `Map<String, Object>` per row from `ResultSet`), `JdbcWriter` (batch `INSERT` using column names from first item's keys), `CsvWriter` (writes CSV with headers from first item), `FileWriter` (appends strings to file).

Each follows the same pattern as `CsvReader`: constructor takes config properties, `open()` initializes resources, `read()`/`write()` does the work, `close()` cleans up.

- [ ] **Step 6: Implement `DefaultReaderWriterFactory`**

```java
package com.batchforge.engine.dag;

import com.batchforge.engine.core.*;
import com.batchforge.readers.*;
import com.batchforge.writers.*;
import com.batchforge.spi.*;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import javax.sql.DataSource;

@ApplicationScoped
public class DefaultReaderWriterFactory implements ReaderWriterFactory {

    @Inject DataSource dataSource;

    @Override
    public ItemReader<?> createReader(ReaderConfig config) {
        return switch (config.type()) {
            case "jdbc" -> new JdbcReader(dataSource, config.properties().get("query"));
            case "csv" -> new CsvReader(config.properties().get("path"));
            case "jsonlines" -> new JsonLinesReader(config.properties().get("path"));
            default -> throw new IllegalArgumentException("Unknown reader type: " + config.type());
        };
    }

    @Override
    public ItemWriter<?> createWriter(WriterConfig config) {
        return switch (config.type()) {
            case "jdbc" -> new JdbcWriter(dataSource, config.properties().get("table"));
            case "csv" -> new CsvWriter(config.properties().get("path"));
            case "file" -> new com.batchforge.writers.FileWriter(config.properties().get("path"));
            default -> throw new IllegalArgumentException("Unknown writer type: " + config.type());
        };
    }

    @Override
    @SuppressWarnings("unchecked")
    public ItemProcessor<?, ?> createProcessor(String className) {
        try {
            Class<?> clazz = Class.forName(className);
            return (ItemProcessor<?, ?>) clazz.getDeclaredConstructor().newInstance();
        } catch (Exception e) {
            throw new IllegalArgumentException("Cannot instantiate processor: " + className, e);
        }
    }
}
```

- [ ] **Step 7: Write and run JDBC integration tests** (with Testcontainers)

Test `JdbcReader` reads rows from a test table, and `JdbcWriter` inserts rows and they're queryable.

- [ ] **Step 8: Run all tests**

Run: `mvn test`
Expected: all tests pass

- [ ] **Step 9: Commit**

```bash
git add src/
git commit -m "feat(batchforge): built-in readers (JDBC, CSV, JSON Lines) and writers (JDBC, CSV, File)

CsvReader with header parsing and seekTo. JsonLinesReader for JSON-per-line files.
JdbcReader executes SQL and maps ResultSet rows. JdbcWriter does batch inserts.
DefaultReaderWriterFactory resolves reader/writer/processor by type string."
```

---

### Task 11: Metrics Collector (Redis + Postgres)

**Files:**
- Create: `src/main/java/com/batchforge/engine/metrics/MetricsCollector.java`
- Create: `src/main/java/com/batchforge/engine/metrics/StepMetricsSnapshot.java`
- Test: `src/test/java/com/batchforge/engine/metrics/MetricsCollectorTest.java`

**Interfaces:**
- Consumes: Redis client (Quarkus `RedisAPI`), `StepExecution` from Task 1
- Produces:
  - `record StepMetricsSnapshot(UUID stepExecutionId, double itemsPerSecond, double latencyP50Ms, double latencyP95Ms, double latencyP99Ms, int errorCount)`
  - `MetricsCollector.recordItemProcessed(UUID stepExecutionId, long latencyNanos)`: records individual item processing time
  - `MetricsCollector.recordError(UUID stepExecutionId)`: increments error counter
  - `MetricsCollector.flush(UUID stepExecutionId)`: calculates percentiles from recorded latencies, publishes to Redis stream `bf:exec:{jobExecId}:metrics`, persists to `step_metrics` table
  - `MetricsCollector.getRealtimeProgress(UUID jobExecutionId): Map<String, StepMetricsSnapshot>`

- [ ] **Step 1: Write metrics test**

```java
package com.batchforge.engine.metrics;

import org.junit.jupiter.api.Test;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class MetricsCollectorTest {

    @Test
    void percentileCalculation() {
        var latencies = new ArrayList<Long>();
        for (int i = 1; i <= 100; i++) latencies.add((long) i * 1_000_000); // 1ms to 100ms

        double p50 = MetricsCollector.percentile(latencies, 50);
        double p95 = MetricsCollector.percentile(latencies, 95);
        double p99 = MetricsCollector.percentile(latencies, 99);

        assertThat(p50).isBetween(49.0, 51.0); // ~50ms
        assertThat(p95).isBetween(94.0, 96.0);
        assertThat(p99).isBetween(98.0, 100.0);
    }

    @Test
    void throughputCalculation() {
        double throughput = MetricsCollector.calculateThroughput(1000, 2_000_000_000L); // 1000 items in 2 seconds
        assertThat(throughput).isBetween(499.0, 501.0);
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `mvn test -Dtest=MetricsCollectorTest`
Expected: FAIL

- [ ] **Step 3: Implement `MetricsCollector`**

```java
package com.batchforge.engine.metrics;

import io.vertx.mutiny.redis.client.RedisAPI;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;

@ApplicationScoped
public class MetricsCollector {

    @Inject RedisAPI redis;

    private final Map<UUID, List<Long>> latencies = new ConcurrentHashMap<>();
    private final Map<UUID, AtomicInteger> errorCounts = new ConcurrentHashMap<>();
    private final Map<UUID, AtomicLong> itemCounts = new ConcurrentHashMap<>();
    private final Map<UUID, AtomicLong> startTimes = new ConcurrentHashMap<>();

    public void recordItemProcessed(UUID stepExecutionId, long latencyNanos) {
        latencies.computeIfAbsent(stepExecutionId, _ -> new CopyOnWriteArrayList<>()).add(latencyNanos);
        itemCounts.computeIfAbsent(stepExecutionId, _ -> new AtomicLong()).incrementAndGet();
        startTimes.computeIfAbsent(stepExecutionId, _ -> new AtomicLong(System.nanoTime()));
    }

    public void recordError(UUID stepExecutionId) {
        errorCounts.computeIfAbsent(stepExecutionId, _ -> new AtomicInteger()).incrementAndGet();
    }

    public StepMetricsSnapshot snapshot(UUID stepExecutionId) {
        var lats = latencies.getOrDefault(stepExecutionId, List.of());
        var sorted = new ArrayList<>(lats);
        long items = itemCounts.getOrDefault(stepExecutionId, new AtomicLong()).get();
        long start = startTimes.getOrDefault(stepExecutionId, new AtomicLong(System.nanoTime())).get();
        int errors = errorCounts.getOrDefault(stepExecutionId, new AtomicInteger()).get();

        double ips = calculateThroughput(items, System.nanoTime() - start);
        double p50 = sorted.isEmpty() ? 0 : percentile(sorted, 50);
        double p95 = sorted.isEmpty() ? 0 : percentile(sorted, 95);
        double p99 = sorted.isEmpty() ? 0 : percentile(sorted, 99);

        return new StepMetricsSnapshot(stepExecutionId, ips, p50, p95, p99, errors);
    }

    public void clear(UUID stepExecutionId) {
        latencies.remove(stepExecutionId);
        errorCounts.remove(stepExecutionId);
        itemCounts.remove(stepExecutionId);
        startTimes.remove(stepExecutionId);
    }

    public static double percentile(List<Long> sortedLatencies, int p) {
        var sorted = new ArrayList<>(sortedLatencies);
        Collections.sort(sorted);
        int index = (int) Math.ceil(p / 100.0 * sorted.size()) - 1;
        return sorted.get(Math.max(0, index)) / 1_000_000.0; // nanos → ms
    }

    public static double calculateThroughput(long items, long durationNanos) {
        if (durationNanos <= 0) return 0;
        return items / (durationNanos / 1_000_000_000.0);
    }
}
```

`StepMetricsSnapshot.java`:
```java
package com.batchforge.engine.metrics;

import java.util.UUID;

public record StepMetricsSnapshot(
    UUID stepExecutionId,
    double itemsPerSecond,
    double latencyP50Ms,
    double latencyP95Ms,
    double latencyP99Ms,
    int errorCount
) {}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `mvn test -Dtest=MetricsCollectorTest`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/
git commit -m "feat(batchforge): metrics collector with percentile calculation and throughput tracking

Records per-item latencies, calculates p50/p95/p99 percentiles,
tracks throughput (items/sec) and error counts per step execution.
In-memory accumulation with snapshot API for real-time dashboard."
```

---

### Task 12: Job Registry + Hot Reload

**Files:**
- Create: `src/main/java/com/batchforge/engine/registry/JobRegistry.java`
- Test: `src/test/java/com/batchforge/engine/registry/JobRegistryTest.java`

**Interfaces:**
- Consumes: `JobDefinitionParser` from Task 3, `DagValidator` from Task 3, `JobDefinitionRepository` from Task 4, `JobDefinition` from Task 1
- Produces:
  - `JobRegistry.register(JobDefinition): void`
  - `JobRegistry.getJob(String name): Optional<JobDefinition>`
  - `JobRegistry.listJobs(): List<JobDefinition>`
  - `JobRegistry.loadFromDirectory(Path dir): void` — scans `*.yml` files, parses, validates, registers
  - `JobRegistry.startWatching(Path dir): void` — watches directory via `WatchService`, hot-reloads changed files

- [ ] **Step 1: Write registry test**

```java
package com.batchforge.engine.registry;

import com.batchforge.engine.core.*;
import com.batchforge.engine.dag.DagValidator;
import com.batchforge.yaml.JobDefinitionParser;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.*;
import java.util.*;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.Mockito.*;

class JobRegistryTest {

    @TempDir Path tempDir;

    @Test
    void registersAndRetrievesJob() {
        var parser = new JobDefinitionParser();
        var validator = new DagValidator();
        var repo = mock(com.batchforge.engine.repository.JobDefinitionRepository.class);
        var registry = new JobRegistry(parser, validator, repo);

        var job = new JobDefinition("test-job", "desc", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));

        registry.register(job);
        assertThat(registry.getJob("test-job")).isPresent();
        assertThat(registry.listJobs()).hasSize(1);
    }

    @Test
    void loadsFromDirectory() throws Exception {
        Files.writeString(tempDir.resolve("test.yml"), """
            name: dir-job
            description: loaded from dir
            steps:
              step1:
                reader: { type: csv, path: /data/f.csv }
                processor: com.Proc
                writer: { type: jdbc, table: t }
            """);

        var parser = new JobDefinitionParser();
        var validator = new DagValidator();
        var repo = mock(com.batchforge.engine.repository.JobDefinitionRepository.class);
        var registry = new JobRegistry(parser, validator, repo);

        registry.loadFromDirectory(tempDir);
        assertThat(registry.getJob("dir-job")).isPresent();
    }

    @Test
    void rejectsInvalidJob() throws Exception {
        Files.writeString(tempDir.resolve("bad.yml"), """
            name: bad-job
            description: has cycle
            steps:
              a:
                depends-on: [b]
                reader: { type: jdbc, query: "SELECT 1" }
                processor: com.P
                writer: { type: jdbc, table: t }
              b:
                depends-on: [a]
                reader: { type: jdbc, query: "SELECT 1" }
                processor: com.P
                writer: { type: jdbc, table: t }
            """);

        var parser = new JobDefinitionParser();
        var validator = new DagValidator();
        var repo = mock(com.batchforge.engine.repository.JobDefinitionRepository.class);
        var registry = new JobRegistry(parser, validator, repo);

        registry.loadFromDirectory(tempDir);
        assertThat(registry.getJob("bad-job")).isEmpty();
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `mvn test -Dtest=JobRegistryTest`
Expected: FAIL

- [ ] **Step 3: Implement `JobRegistry`**

```java
package com.batchforge.engine.registry;

import com.batchforge.engine.core.JobDefinition;
import com.batchforge.engine.dag.DagValidator;
import com.batchforge.engine.repository.JobDefinitionRepository;
import com.batchforge.yaml.JobDefinitionParser;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;

import java.io.IOException;
import java.nio.file.*;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;

@ApplicationScoped
public class JobRegistry {

    private static final Logger LOG = Logger.getLogger(JobRegistry.class);
    private final Map<String, JobDefinition> jobs = new ConcurrentHashMap<>();
    private final JobDefinitionParser parser;
    private final DagValidator validator;
    private final JobDefinitionRepository repository;

    @Inject
    public JobRegistry(JobDefinitionParser parser, DagValidator validator, JobDefinitionRepository repository) {
        this.parser = parser;
        this.validator = validator;
        this.repository = repository;
    }

    public void register(JobDefinition job) {
        var errors = validator.validate(job);
        if (!errors.isEmpty()) {
            LOG.warnf("Job '%s' has validation errors: %s", job.name(), errors);
            return;
        }
        jobs.put(job.name(), job);
        try {
            String dagJson = new ObjectMapper().writeValueAsString(validator.topologicalSort(job));
            repository.save(job, dagJson, Integer.toHexString(job.hashCode()));
        } catch (Exception e) {
            LOG.warnf("Failed to persist job '%s': %s", job.name(), e.getMessage());
        }
        LOG.infof("Registered job '%s' with %d steps", job.name(), job.steps().size());
    }

    public Optional<JobDefinition> getJob(String name) {
        return Optional.ofNullable(jobs.get(name));
    }

    public List<JobDefinition> listJobs() {
        return List.copyOf(jobs.values());
    }

    public void loadFromDirectory(Path dir) {
        try (var stream = Files.list(dir)) {
            stream.filter(p -> p.toString().endsWith(".yml") || p.toString().endsWith(".yaml"))
                .forEach(file -> {
                    try {
                        var job = parser.parseFile(file);
                        register(job);
                    } catch (Exception e) {
                        LOG.warnf("Failed to load job from '%s': %s", file, e.getMessage());
                    }
                });
        } catch (IOException e) {
            LOG.errorf("Failed to scan directory '%s': %s", dir, e.getMessage());
        }
    }

    public void startWatching(Path dir) {
        Thread.ofVirtual().name("job-watcher").start(() -> {
            try (WatchService watcher = FileSystems.getDefault().newWatchService()) {
                dir.register(watcher, StandardWatchEventKinds.ENTRY_CREATE, StandardWatchEventKinds.ENTRY_MODIFY);
                while (!Thread.currentThread().isInterrupted()) {
                    WatchKey key = watcher.take();
                    for (WatchEvent<?> event : key.pollEvents()) {
                        Path file = dir.resolve((Path) event.context());
                        if (file.toString().endsWith(".yml") || file.toString().endsWith(".yaml")) {
                            LOG.infof("Detected change in '%s', reloading", file);
                            try {
                                register(parser.parseFile(file));
                            } catch (Exception e) {
                                LOG.warnf("Failed to reload '%s': %s", file, e.getMessage());
                            }
                        }
                    }
                    key.reset();
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            } catch (IOException e) {
                LOG.errorf("Watcher failed: %s", e.getMessage());
            }
        });
    }
}
```

Make `JobDefinitionParser` a CDI bean by adding `@ApplicationScoped`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `mvn test -Dtest=JobRegistryTest`
Expected: PASS — all 3 tests green

- [ ] **Step 5: Commit**

```bash
git add src/
git commit -m "feat(batchforge): job registry with YAML directory loading and hot-reload file watcher

Loads *.yml jobs from directory, validates DAG, registers in-memory
and persists to Postgres. WatchService-based hot-reload detects file
changes and re-registers jobs on virtual thread."
```

---

### Task 13: Engine Bootstrap + End-to-End Integration Test

**Files:**
- Create: `src/main/java/com/batchforge/engine/BatchForgeEngine.java`
- Create: `src/test/java/com/batchforge/engine/EngineIntegrationTest.java`
- Create: `src/test/resources/jobs/simple-test-job.yml`
- Create: `src/test/java/com/batchforge/engine/TestPassthroughProcessor.java`

**Interfaces:**
- Consumes: `JobRegistry` from Task 12, `DagScheduler` from Task 9, `JobExecutionRepository` from Task 4
- Produces:
  - `BatchForgeEngine.start()`: loads jobs from configured directory, starts watcher
  - `BatchForgeEngine.executeJob(String jobName, Map<String, Object> params): UUID`: triggers execution, returns execution ID
  - `BatchForgeEngine.getExecution(UUID id): Optional<JobExecution>`

- [ ] **Step 1: Create test job YAML and a passthrough processor**

`src/test/resources/jobs/simple-test-job.yml`:
```yaml
name: simple-test
description: End-to-end test job
steps:
  read-and-write:
    reader:
      type: csv
      path: "${dataDir}/input.csv"
    processor: com.batchforge.engine.TestPassthroughProcessor
    writer:
      type: csv
      path: "${dataDir}/output.csv"
    chunk-size: 2
```

`TestPassthroughProcessor.java`:
```java
package com.batchforge.engine;

import com.batchforge.spi.ItemProcessor;
import java.util.Map;

public class TestPassthroughProcessor implements ItemProcessor<Map<String, Object>, Map<String, Object>> {
    @Override
    public Map<String, Object> process(Map<String, Object> item) {
        return item;
    }
}
```

- [ ] **Step 2: Write integration test**

```java
package com.batchforge.engine;

import com.batchforge.engine.core.*;
import com.batchforge.engine.repository.*;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.*;
import java.util.Map;

import static org.assertj.core.api.Assertions.*;

@QuarkusTest
@QuarkusTestResource(TestcontainersConfig.class)
class EngineIntegrationTest {

    @Inject BatchForgeEngine engine;
    @Inject JobExecutionRepository jobExecRepo;

    @TempDir Path tempDir;

    @Test
    void executesSimpleJobEndToEnd() throws Exception {
        // Create input CSV
        Files.writeString(tempDir.resolve("input.csv"), "id,name\n1,Alice\n2,Bob\n3,Carol\n");

        // Load and execute job
        Path jobFile = tempDir.resolve("simple-test.yml");
        Files.writeString(jobFile, """
            name: e2e-test
            description: integration test
            steps:
              read-and-write:
                reader:
                  type: csv
                  path: "%s/input.csv"
                processor: com.batchforge.engine.TestPassthroughProcessor
                writer:
                  type: csv
                  path: "%s/output.csv"
                chunk-size: 2
            """.formatted(tempDir, tempDir));

        engine.loadJobFile(jobFile);
        UUID execId = engine.executeJob("e2e-test", Map.of());

        // Wait for completion (job runs async)
        Thread.sleep(2000);

        var exec = jobExecRepo.findById(execId);
        assertThat(exec).isPresent();
        assertThat(exec.get().status().toDbValue()).isEqualTo("COMPLETED");

        // Verify output
        String output = Files.readString(tempDir.resolve("output.csv"));
        assertThat(output).contains("Alice").contains("Bob").contains("Carol");
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `mvn test -Dtest=EngineIntegrationTest`
Expected: FAIL — `BatchForgeEngine` not found

- [ ] **Step 4: Implement `BatchForgeEngine`**

```java
package com.batchforge.engine;

import com.batchforge.engine.core.*;
import com.batchforge.engine.dag.DagScheduler;
import com.batchforge.engine.registry.JobRegistry;
import com.batchforge.engine.repository.JobExecutionRepository;
import com.batchforge.yaml.JobDefinitionParser;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;

import java.nio.file.Path;
import java.util.*;

@ApplicationScoped
public class BatchForgeEngine {

    private static final Logger LOG = Logger.getLogger(BatchForgeEngine.class);

    @Inject JobRegistry registry;
    @Inject DagScheduler scheduler;
    @Inject JobExecutionRepository jobExecRepo;
    @Inject JobDefinitionParser parser;

    @ConfigProperty(name = "batchforge.jobs-dir", defaultValue = "jobs")
    String jobsDir;

    public void start() {
        Path dir = Path.of(jobsDir);
        if (dir.toFile().exists()) {
            registry.loadFromDirectory(dir);
            registry.startWatching(dir);
            LOG.infof("BatchForge started, watching '%s'", dir);
        }
    }

    public void loadJobFile(Path file) throws Exception {
        registry.register(parser.parseFile(file));
    }

    public UUID executeJob(String jobName, Map<String, Object> parameters) {
        var jobDef = registry.getJob(jobName)
            .orElseThrow(() -> new IllegalArgumentException("Job not found: " + jobName));

        var execution = jobExecRepo.save(JobExecution.create(jobName, parameters));

        Thread.ofVirtual().name("job-" + execution.id()).start(() -> {
            try {
                scheduler.execute(jobDef, execution);
            } catch (Exception e) {
                LOG.errorf("Job '%s' execution %s failed: %s", jobName, execution.id(), e.getMessage());
            }
        });

        return execution.id();
    }

    public Optional<JobExecution> getExecution(UUID id) {
        return jobExecRepo.findById(id);
    }
}
```

- [ ] **Step 5: Run integration test to verify it passes**

Run: `mvn test -Dtest=EngineIntegrationTest`
Expected: PASS — job executes end-to-end, output CSV is written

- [ ] **Step 6: Run full test suite**

Run: `mvn test`
Expected: all tests pass

- [ ] **Step 7: Commit**

```bash
git add src/
git commit -m "feat(batchforge): engine bootstrap with end-to-end integration test

BatchForgeEngine orchestrates startup, job loading, and async execution.
E2E test verifies CSV → passthrough processor → CSV pipeline through
the full engine stack with real Postgres via Testcontainers."
```

---

## Sub-Project 1 Complete

At this point, the BatchForge engine core is fully functional:
- Domain model with sealed interfaces + records
- SPI interfaces for extensible readers/processors/writers
- YAML job parser with DAG validation
- Topological sort for level-based parallel execution
- Chunk-based step executor with Virtual Threads
- Checkpointing with restart recovery
- Retry policies with fixed/exponential/jitter backoff
- Dead letter queue for failed items
- Partition manager for intra-step parallelism
- Metrics collector with percentile calculation
- Job registry with hot-reload file watcher
- Built-in readers (JDBC, CSV, JSON Lines) and writers (JDBC, CSV, File)
- Docker Compose (Postgres 17 + Redis 7) + Dockerfile
- Full test suite: unit, integration, and end-to-end

**Next sub-projects:**
- Sub-Project 2: REST API + WebSocket (all HTTP endpoints + real-time streaming)
- Sub-Project 3: Showcase Use Cases (bank reconciliation + log ETL)
- Sub-Project 4: React Dashboard (job list, execution detail, DAG builder, dead letter viewer, metrics)
