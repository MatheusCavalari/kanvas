package com.batchforge.engine.executor;

import com.batchforge.engine.checkpoint.CheckpointManager;
import com.batchforge.engine.core.RetryConfig;
import com.batchforge.engine.deadletter.DeadLetterManager;
import com.batchforge.engine.retry.RetryPolicy;
import com.batchforge.spi.*;
import org.junit.jupiter.api.Test;

import java.util.*;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicLong;

import static org.assertj.core.api.Assertions.*;

class StepExecutorTest {

    @Test
    void processesAllItemsInChunks() {
        var reader = new ListReader<>(List.of("a", "b", "c", "d", "e"));
        ItemProcessor<String, String> processor = s -> s.toUpperCase();
        var writer = new CollectingWriter<String>();
        var checkpointMgr = new FakeCheckpointManager();
        var deadLetterMgr = new FakeDeadLetterManager();
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of());

        var result = new StepExecutor(checkpointMgr, deadLetterMgr)
            .execute(ctx, reader, processor, writer, 2, RetryPolicy.from(RetryConfig.none()));

        assertThat(writer.written).containsExactly("A", "B", "C", "D", "E");
        assertThat(result.itemsRead()).isEqualTo(5);
        assertThat(result.itemsWritten()).isEqualTo(5);
        assertThat(result.itemsFailed()).isZero();
        assertThat(checkpointMgr.saveCount).isEqualTo(3);
    }

    @Test
    void failedItemsGoToDeadLetterAndChunkContinues() {
        var reader = new ListReader<>(List.of("ok", "fail", "ok2"));
        ItemProcessor<String, String> processor = s -> {
            if (s.equals("fail")) throw new ProcessingException("bad item");
            return s.toUpperCase();
        };
        var writer = new CollectingWriter<String>();
        var checkpointMgr = new FakeCheckpointManager();
        var deadLetterMgr = new FakeDeadLetterManager();
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of());

        var result = new StepExecutor(checkpointMgr, deadLetterMgr)
            .execute(ctx, reader, processor, writer, 10, RetryPolicy.from(RetryConfig.none()));

        assertThat(writer.written).containsExactly("OK", "OK2");
        assertThat(result.itemsFailed()).isEqualTo(1);
        assertThat(deadLetterMgr.sent).hasSize(1);
        var call = deadLetterMgr.sent.get(0);
        assertThat(call.stepExecutionId()).isEqualTo(ctx.stepExecutionId());
        assertThat(call.stepName()).isEqualTo("s1");
        assertThat(call.item()).isEqualTo("fail");
        assertThat(call.attempt()).isEqualTo(1);
    }

    @Test
    void resumesFromCheckpointOffset() {
        var reader = new ListReader<>(List.of("a", "b", "c", "d", "e"));
        ItemProcessor<String, String> processor = s -> s.toUpperCase();
        var writer = new CollectingWriter<String>();
        var checkpointMgr = new FakeCheckpointManager();
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of());
        checkpointMgr.save(ctx.stepExecutionId(), "main", 3, Map.of());
        checkpointMgr.saveCount = 0;

        var result = new StepExecutor(checkpointMgr, new FakeDeadLetterManager())
            .execute(ctx, reader, processor, writer, 10, RetryPolicy.from(RetryConfig.none()));

        assertThat(writer.written).containsExactly("D", "E");
        assertThat(result.itemsRead()).isEqualTo(2);
    }

    // Fakes
    static class FakeCheckpointManager extends CheckpointManager {
        final Map<String, Long> offsets = new HashMap<>();
        int saveCount;

        private static String key(UUID id, String partition) { return id + ":" + partition; }

        @Override public void save(UUID id, String partition, long offset, Map<String, Object> metadata) {
            offsets.put(key(id, partition), offset);
            saveCount++;
        }

        @Override public OptionalLong getLastOffset(UUID id, String partition) {
            Long v = offsets.get(key(id, partition));
            return v == null ? OptionalLong.empty() : OptionalLong.of(v);
        }
    }

    record DeadLetterCall(UUID stepExecutionId, String stepName, Object item, Exception error, int attempt) {}

    static class FakeDeadLetterManager extends DeadLetterManager {
        final List<DeadLetterCall> sent = new CopyOnWriteArrayList<>();

        @Override public void send(UUID id, String stepName, Object item, Exception error, int attempt) {
            sent.add(new DeadLetterCall(id, stepName, item, error, attempt));
        }
    }

    static class ListReader<T> implements ItemReader<T> {
        private final List<T> items;
        private final AtomicLong offset = new AtomicLong(0);

        ListReader(List<T> items) { this.items = new ArrayList<>(items); }

        @Override public void open(ExecutionContext context) {}
        @Override public List<T> read(int chunkSize) {
            int from = (int) offset.get();
            int to = Math.min(from + chunkSize, items.size());
            var chunk = items.subList(from, to);
            offset.set(to);
            return new ArrayList<>(chunk);
        }
        @Override public boolean hasMore() { return offset.get() < items.size(); }
        @Override public void seekTo(long off) { offset.set(off); }
        @Override public void close() {}
    }

    static class CollectingWriter<T> implements ItemWriter<T> {
        final List<T> written = new CopyOnWriteArrayList<>();
        @Override public void open(ExecutionContext context) {}
        @Override public void write(List<T> items) { written.addAll(items); }
        @Override public void close() {}
    }
}
