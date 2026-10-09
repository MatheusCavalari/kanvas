package com.batchforge.engine.metrics;

import io.vertx.mutiny.redis.client.RedisAPI;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;

@ApplicationScoped
public class MetricsCollector {

    @Inject RedisAPI redis;

    private final Map<UUID, List<Long>> latencies = new ConcurrentHashMap<>();
    private final Map<UUID, AtomicInteger> errorCounts = new ConcurrentHashMap<>();
    private final Map<UUID, AtomicLong> itemCounts = new ConcurrentHashMap<>();
    private final Map<UUID, AtomicLong> startTimes = new ConcurrentHashMap<>();

    public void recordItemProcessed(UUID stepExecutionId, long latencyNanos) {
        latencies.computeIfAbsent(stepExecutionId, _ -> new CopyOnWriteArrayList<>()).add(latencyNanos);
        itemCounts.computeIfAbsent(stepExecutionId, _ -> new AtomicLong()).incrementAndGet();
        startTimes.computeIfAbsent(stepExecutionId, _ -> new AtomicLong(System.nanoTime()));
    }

    public void recordError(UUID stepExecutionId) {
        errorCounts.computeIfAbsent(stepExecutionId, _ -> new AtomicInteger()).incrementAndGet();
    }

    public StepMetricsSnapshot snapshot(UUID stepExecutionId) {
        var lats = latencies.getOrDefault(stepExecutionId, List.of());
        var sorted = new ArrayList<>(lats);
        long items = itemCounts.getOrDefault(stepExecutionId, new AtomicLong()).get();
        long start = startTimes.getOrDefault(stepExecutionId, new AtomicLong(System.nanoTime())).get();
        int errors = errorCounts.getOrDefault(stepExecutionId, new AtomicInteger()).get();

        double ips = calculateThroughput(items, System.nanoTime() - start);
        double p50 = sorted.isEmpty() ? 0 : percentile(sorted, 50);
        double p95 = sorted.isEmpty() ? 0 : percentile(sorted, 95);
        double p99 = sorted.isEmpty() ? 0 : percentile(sorted, 99);

        return new StepMetricsSnapshot(stepExecutionId, ips, p50, p95, p99, errors);
    }

    public void clear(UUID stepExecutionId) {
        latencies.remove(stepExecutionId);
        errorCounts.remove(stepExecutionId);
        itemCounts.remove(stepExecutionId);
        startTimes.remove(stepExecutionId);
    }

    public static double percentile(List<Long> sortedLatencies, int p) {
        var sorted = new ArrayList<>(sortedLatencies);
        Collections.sort(sorted);
        int index = (int) Math.ceil(p / 100.0 * sorted.size()) - 1;
        return sorted.get(Math.max(0, index)) / 1_000_000.0; // nanos → ms
    }

    public static double calculateThroughput(long items, long durationNanos) {
        if (durationNanos <= 0) return 0;
        return items / (durationNanos / 1_000_000_000.0);
    }
}
