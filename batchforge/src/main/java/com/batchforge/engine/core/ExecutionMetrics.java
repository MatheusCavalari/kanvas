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
