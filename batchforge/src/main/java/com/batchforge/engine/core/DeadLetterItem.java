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
