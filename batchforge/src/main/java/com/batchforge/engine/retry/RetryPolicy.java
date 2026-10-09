package com.batchforge.engine.retry;

import com.batchforge.engine.core.RetryConfig;
import java.time.Duration;
import java.util.Set;
import java.util.stream.Collectors;

public final class RetryPolicy {
    private final int maxAttempts;
    private final BackoffStrategy backoff;
    private final Set<String> retryableExceptions;

    private RetryPolicy(int maxAttempts, BackoffStrategy backoff, Set<String> retryableExceptions) {
        this.maxAttempts = maxAttempts;
        this.backoff = backoff;
        this.retryableExceptions = retryableExceptions;
    }

    public static RetryPolicy from(RetryConfig config) {
        BackoffStrategy backoff = switch (config.backoff()) {
            case "fixed" -> new FixedBackoff(config.baseDelay());
            case "exponential" -> new ExponentialBackoff(config.baseDelay(), config.maxDelay());
            case "exponential-jitter" -> new ExponentialJitterBackoff(config.baseDelay(), config.maxDelay());
            default -> new FixedBackoff(config.baseDelay());
        };
        return new RetryPolicy(config.maxAttempts(), backoff,
            config.retryableExceptions().stream().collect(Collectors.toSet()));
    }

    public boolean shouldRetry(Exception e, int attempt) {
        if (attempt >= maxAttempts) return false;
        if (retryableExceptions.isEmpty()) return true;
        return retryableExceptions.contains(e.getClass().getName());
    }

    public Duration getDelay(int attempt) {
        return backoff.nextDelay(attempt);
    }
}
