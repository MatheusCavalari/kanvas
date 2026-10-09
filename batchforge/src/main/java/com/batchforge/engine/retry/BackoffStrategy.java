package com.batchforge.engine.retry;

import java.time.Duration;

public interface BackoffStrategy {
    Duration nextDelay(int attempt);
}
