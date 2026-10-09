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
