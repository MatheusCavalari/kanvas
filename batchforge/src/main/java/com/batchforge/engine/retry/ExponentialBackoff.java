package com.batchforge.engine.retry;

import java.time.Duration;

public record ExponentialBackoff(Duration baseDelay, Duration maxDelay) implements BackoffStrategy {
    @Override
    public Duration nextDelay(int attempt) {
        long delayMs = baseDelay.toMillis() * (1L << (attempt - 1));
        return Duration.ofMillis(Math.min(delayMs, maxDelay.toMillis()));
    }
}
