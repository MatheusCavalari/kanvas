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
