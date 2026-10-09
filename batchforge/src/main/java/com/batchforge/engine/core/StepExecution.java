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
