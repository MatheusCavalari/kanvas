package com.batchforge.engine.retry;

import java.time.Duration;
import java.util.concurrent.ThreadLocalRandom;

public record ExponentialJitterBackoff(Duration baseDelay, Duration maxDelay) implements BackoffStrategy {
    @Override
    public Duration nextDelay(int attempt) {
        long expMs = baseDelay.toMillis() * (1L << (attempt - 1));
        long cappedMs = Math.min(expMs, maxDelay.toMillis());
        long jitteredMs = ThreadLocalRandom.current().nextLong(cappedMs / 2, cappedMs + 1);
        return Duration.ofMillis(jitteredMs);
    }
}
