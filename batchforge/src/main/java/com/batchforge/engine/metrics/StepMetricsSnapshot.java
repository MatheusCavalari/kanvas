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
