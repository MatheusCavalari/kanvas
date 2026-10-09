package com.batchforge.engine.core;

import org.junit.jupiter.api.Test;
import java.time.Duration;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class DomainModelTest {

    @Test
    void executionStatusRoundtripsToDbValue() {
        var statuses = List.of(
            new ExecutionStatus.Pending(),
            new ExecutionStatus.Running(),
            new ExecutionStatus.Completed(),
            new ExecutionStatus.Failed("timeout"),
            new ExecutionStatus.Restarting(),
            new ExecutionStatus.Abandoned("max restarts")
        );
        for (var status : statuses) {
            String db = status.toDbValue();
            String msg = status instanceof ExecutionStatus.Failed f ? f.message()
                       : status instanceof ExecutionStatus.Abandoned a ? a.message() : null;
            ExecutionStatus restored = ExecutionStatus.fromDbValue(db, msg);
            assertThat(restored).isEqualTo(status);
        }
    }

    @Test
    void stepStatusRoundtripsToDbValue() {
        var statuses = List.of(
            new StepStatus.Waiting(),
            new StepStatus.Ready(),
            new StepStatus.StepRunning(),
            new StepStatus.StepCompleted(),
            new StepStatus.StepFailed("NPE"),
            new StepStatus.Skipped("dependency failed")
        );
        for (var status : statuses) {
            String db = status.toDbValue();
            String msg = status instanceof StepStatus.StepFailed f ? f.message()
                       : status instanceof StepStatus.Skipped s ? s.reason() : null;
            StepStatus restored = StepStatus.fromDbValue(db, msg);
            assertThat(restored).isEqualTo(status);
        }
    }

    @Test
    void jobDefinitionRequiresAtLeastOneStep() {
        assertThatThrownBy(() -> new JobDefinition("test", "desc", null, Map.of(), Map.of()))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("at least one step");
    }

    @Test
    void stepDefinitionDefaultsChunkSizeAndPartitions() {
        var step = new StepDefinition("s1",
            new ReaderConfig("jdbc", Map.of()), "com.Proc",
            new WriterConfig("jdbc", Map.of()), 0, 0, null, null, false);
        assertThat(step.chunkSize()).isEqualTo(100);
        assertThat(step.partitions()).isEqualTo(1);
        assertThat(step.dependsOn()).isEmpty();
        assertThat(step.retry()).isEqualTo(RetryConfig.none());
    }

    @Test
    void jobExecutionFactoryMethodCreatesWithPendingStatus() {
        var exec = JobExecution.create("my-job", Map.of("key", "val"));
        assertThat(exec.id()).isNotNull();
        assertThat(exec.jobName()).isEqualTo("my-job");
        assertThat(exec.status()).isInstanceOf(ExecutionStatus.Pending.class);
        assertThat(exec.parameters()).containsEntry("key", "val");
    }

    @Test
    void exhaustivePatternMatchOnExecutionStatus() {
        ExecutionStatus status = new ExecutionStatus.Failed("boom");
        String label = switch (status) {
            case ExecutionStatus.Pending _ -> "pending";
            case ExecutionStatus.Running _ -> "running";
            case ExecutionStatus.Completed _ -> "completed";
            case ExecutionStatus.Failed f -> "failed: " + f.message();
            case ExecutionStatus.Restarting _ -> "restarting";
            case ExecutionStatus.Abandoned a -> "abandoned: " + a.message();
        };
        assertThat(label).isEqualTo("failed: boom");
    }

    @Test
    void retryConfigDefaults() {
        var def = RetryConfig.defaultConfig();
        assertThat(def.maxAttempts()).isEqualTo(3);
        assertThat(def.backoff()).isEqualTo("exponential");
        assertThat(def.baseDelay()).isEqualTo(Duration.ofSeconds(1));
    }
}
