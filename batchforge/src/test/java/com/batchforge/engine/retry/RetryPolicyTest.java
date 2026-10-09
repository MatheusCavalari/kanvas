package com.batchforge.engine.retry;

import com.batchforge.engine.core.RetryConfig;
import org.junit.jupiter.api.Test;
import java.net.SocketTimeoutException;
import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.*;

class RetryPolicyTest {

    @Test
    void fixedBackoffReturnsSameDelay() {
        var backoff = new FixedBackoff(Duration.ofSeconds(2));
        assertThat(backoff.nextDelay(1)).isEqualTo(Duration.ofSeconds(2));
        assertThat(backoff.nextDelay(5)).isEqualTo(Duration.ofSeconds(2));
    }

    @Test
    void exponentialBackoffDoublesWithCap() {
        var backoff = new ExponentialBackoff(Duration.ofSeconds(1), Duration.ofSeconds(10));
        assertThat(backoff.nextDelay(1)).isEqualTo(Duration.ofSeconds(1));
        assertThat(backoff.nextDelay(2)).isEqualTo(Duration.ofSeconds(2));
        assertThat(backoff.nextDelay(3)).isEqualTo(Duration.ofSeconds(4));
        assertThat(backoff.nextDelay(4)).isEqualTo(Duration.ofSeconds(8));
        assertThat(backoff.nextDelay(5)).isEqualTo(Duration.ofSeconds(10)); // capped
    }

    @Test
    void exponentialJitterNeverExceedsMax() {
        var backoff = new ExponentialJitterBackoff(Duration.ofSeconds(1), Duration.ofSeconds(10));
        for (int i = 1; i <= 20; i++) {
            assertThat(backoff.nextDelay(i)).isLessThanOrEqualTo(Duration.ofSeconds(10));
        }
    }

    @Test
    void retryPolicyRespectsMaxAttempts() {
        var config = new RetryConfig(3, "fixed", Duration.ofSeconds(1), Duration.ofSeconds(30), List.of());
        var policy = RetryPolicy.from(config);

        assertThat(policy.shouldRetry(new RuntimeException(), 1)).isTrue();
        assertThat(policy.shouldRetry(new RuntimeException(), 3)).isFalse();
    }

    @Test
    void retryPolicyFiltersExceptionTypes() {
        var config = new RetryConfig(3, "fixed", Duration.ofSeconds(1), Duration.ofSeconds(30),
            List.of("java.net.SocketTimeoutException"));
        var policy = RetryPolicy.from(config);

        assertThat(policy.shouldRetry(new SocketTimeoutException(), 1)).isTrue();
        assertThat(policy.shouldRetry(new NullPointerException(), 1)).isFalse();
    }

    @Test
    void noneConfigNeverRetries() {
        var policy = RetryPolicy.from(RetryConfig.none());
        assertThat(policy.shouldRetry(new RuntimeException(), 1)).isFalse();
    }
}
