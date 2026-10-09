package com.batchforge.engine.metrics;

import org.junit.jupiter.api.Test;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class MetricsCollectorTest {

    @Test
    void percentileCalculation() {
        var latencies = new ArrayList<Long>();
        for (int i = 1; i <= 100; i++) latencies.add((long) i * 1_000_000); // 1ms to 100ms

        double p50 = MetricsCollector.percentile(latencies, 50);
        double p95 = MetricsCollector.percentile(latencies, 95);
        double p99 = MetricsCollector.percentile(latencies, 99);

        assertThat(p50).isBetween(49.0, 51.0); // ~50ms
        assertThat(p95).isBetween(94.0, 96.0);
        assertThat(p99).isBetween(98.0, 100.0);
    }

    @Test
    void throughputCalculation() {
        double throughput = MetricsCollector.calculateThroughput(1000, 2_000_000_000L); // 1000 items in 2 seconds
        assertThat(throughput).isBetween(499.0, 501.0);
    }
}
