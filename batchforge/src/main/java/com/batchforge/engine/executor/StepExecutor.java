package com.batchforge.engine.executor;

import com.batchforge.engine.checkpoint.CheckpointManager;
import com.batchforge.engine.deadletter.DeadLetterManager;
import com.batchforge.engine.retry.RetryPolicy;
import com.batchforge.spi.*;

import java.time.Duration;
import java.time.Instant;
import java.util.*;
import java.util.concurrent.StructuredTaskScope;

public class StepExecutor {

    private final CheckpointManager checkpointManager;
    private final DeadLetterManager deadLetterManager;

    public StepExecutor(CheckpointManager checkpointManager, DeadLetterManager deadLetterManager) {
        this.checkpointManager = checkpointManager;
        this.deadLetterManager = deadLetterManager;
    }

    public <I, O> ChunkResult execute(
            ExecutionContext context,
            ItemReader<I> reader,
            ItemProcessor<I, O> processor,
            ItemWriter<O> writer,
            int chunkSize,
            RetryPolicy retryPolicy) {

        Instant start = Instant.now();
        long totalRead = 0, totalWritten = 0, totalFailed = 0;

        reader.open(context);
        writer.open(context);

        try {
            var lastOffset = checkpointManager.getLastOffset(context.stepExecutionId(), context.partitionId());
            if (lastOffset.isPresent()) {
                reader.seekTo(lastOffset.getAsLong());
            }
            long currentOffset = lastOffset.orElse(0);

            while (reader.hasMore()) {
                List<I> chunk = reader.read(chunkSize);
                if (chunk.isEmpty()) break;

                totalRead += chunk.size();

                List<O> processed = new ArrayList<>();
                long chunkFailed = 0;

                record ItemResult<T, R>(R value, Exception error, T original) {}
                try (var scope = StructuredTaskScope.<ItemResult<I, O>>open()) {
                    List<StructuredTaskScope.Subtask<ItemResult<I, O>>> subtasks = new ArrayList<>();

                    for (I item : chunk) {
                        subtasks.add(scope.fork(() -> {
                            try {
                                return new ItemResult<I, O>(processor.process(item), null, item);
                            } catch (Exception e) {
                                return new ItemResult<I, O>(null, e, item);
                            }
                        }));
                    }

                    scope.join();

                    for (var subtask : subtasks) {
                        ItemResult<I, O> result = subtask.get();
                        if (result.error() != null) {
                            int attempt = 1;
                            Exception lastError = result.error();
                            boolean recovered = false;
                            while (retryPolicy.shouldRetry(lastError, attempt)) {
                                Thread.sleep(retryPolicy.getDelay(attempt));
                                attempt++;
                                try {
                                    processed.add(processor.process(result.original()));
                                    recovered = true;
                                    break;
                                } catch (Exception retryError) {
                                    lastError = retryError;
                                }
                            }
                            if (!recovered) {
                                chunkFailed++;
                                deadLetterManager.send(context.stepExecutionId(), context.stepName(),
                                    result.original(), lastError, attempt);
                            }
                        } else {
                            processed.add(result.value());
                        }
                    }
                }

                totalFailed += chunkFailed;

                if (!processed.isEmpty()) {
                    writer.write(processed);
                    totalWritten += processed.size();
                }

                currentOffset += chunk.size();
                checkpointManager.save(context.stepExecutionId(), context.partitionId(), currentOffset, Map.of());
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new RuntimeException("Step execution interrupted", e);
        } finally {
            reader.close();
            writer.close();
        }

        return new ChunkResult((int) totalRead, (int) totalWritten, (int) totalFailed,
            Duration.between(start, Instant.now()));
    }
}
