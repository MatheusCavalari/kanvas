package com.batchforge.spi;

import org.junit.jupiter.api.Test;
import java.util.Map;
import java.util.UUID;

import static org.assertj.core.api.Assertions.*;

class ExecutionContextTest {

    @Test
    void contextExposesAllFields() {
        var jobExecId = UUID.randomUUID();
        var stepExecId = UUID.randomUUID();
        var ctx = new ExecutionContext(
            jobExecId, stepExecId, "load-data", "partition-0",
            Map.of("accountId", "ACC-001"),
            Map.of("query", "SELECT * FROM t")
        );
        assertThat(ctx.jobExecutionId()).isEqualTo(jobExecId);
        assertThat(ctx.stepExecutionId()).isEqualTo(stepExecId);
        assertThat(ctx.stepName()).isEqualTo("load-data");
        assertThat(ctx.partitionId()).isEqualTo("partition-0");
        assertThat(ctx.jobParameters()).containsEntry("accountId", "ACC-001");
        assertThat(ctx.stepProperties()).containsEntry("query", "SELECT * FROM t");
    }

    @Test
    void parameterResolutionReplacesPlaceholders() {
        var ctx = new ExecutionContext(
            UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of("accountId", "ACC-001", "date", "2026-01-15"),
            Map.of()
        );
        String resolved = ctx.resolveTemplate("SELECT * FROM t WHERE account = :accountId AND date = :date");
        assertThat(resolved).isEqualTo("SELECT * FROM t WHERE account = ACC-001 AND date = 2026-01-15");
    }

    @Test
    void parameterResolutionLeavesUnknownPlaceholders() {
        var ctx = new ExecutionContext(
            UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of("accountId", "ACC-001"),
            Map.of()
        );
        String resolved = ctx.resolveTemplate("WHERE account = :accountId AND date = :date");
        assertThat(resolved).isEqualTo("WHERE account = ACC-001 AND date = :date");
    }
}
