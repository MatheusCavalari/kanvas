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
