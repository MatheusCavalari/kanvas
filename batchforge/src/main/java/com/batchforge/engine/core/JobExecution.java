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
