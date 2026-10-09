package com.batchforge.engine.retry;

import java.time.Duration;

public record FixedBackoff(Duration delay) implements BackoffStrategy {
    @Override
    public Duration nextDelay(int attempt) {
        return delay;
    }
}
