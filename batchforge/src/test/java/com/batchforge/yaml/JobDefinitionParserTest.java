package com.batchforge.yaml;

import com.batchforge.engine.core.*;
import org.junit.jupiter.api.Test;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.time.Duration;

import static org.assertj.core.api.Assertions.*;

class JobDefinitionParserTest {

    private final JobDefinitionParser parser = new JobDefinitionParser();

    private String loadYaml(String resource) throws Exception {
        try (InputStream is = getClass().getResourceAsStream(resource)) {
            return new String(is.readAllBytes(), StandardCharsets.UTF_8);
        }
    }

    @Test
    void parsesValidReconciliationJob() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));

        assertThat(job.name()).isEqualTo("bank-reconciliation");
        assertThat(job.schedule()).isEqualTo("0 2 * * *");
        assertThat(job.steps()).hasSize(4);
        assertThat(job.parameters()).containsKey("accountId");
        assertThat(job.parameters().get("accountId").required()).isTrue();
        assertThat(job.parameters().get("date").defaultValue()).isEqualTo("2026-01-15");
    }

    @Test
    void parsesStepWithChunkSizeAndPartitions() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        StepDefinition loadTxn = job.steps().get("load-transactions");

        assertThat(loadTxn.chunkSize()).isEqualTo(1000);
        assertThat(loadTxn.partitions()).isEqualTo(4);
        assertThat(loadTxn.reader().type()).isEqualTo("jdbc");
        assertThat(loadTxn.writer().type()).isEqualTo("jdbc");
        assertThat(loadTxn.processorClass()).isEqualTo("com.batchforge.demo.TransactionNormalizer");
    }

    @Test
    void parsesStepDependencies() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        StepDefinition match = job.steps().get("match");

        assertThat(match.dependsOn()).containsExactlyInAnyOrder("load-transactions", "load-statements");
    }

    @Test
    void parsesRetryConfig() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        RetryConfig retry = job.steps().get("match").retry();

        assertThat(retry.maxAttempts()).isEqualTo(3);
        assertThat(retry.backoff()).isEqualTo("exponential");
        assertThat(retry.baseDelay()).isEqualTo(Duration.ofSeconds(1));
    }

    @Test
    void stepsWithoutRetryGetDefaultNone() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        RetryConfig retry = job.steps().get("load-transactions").retry();

        assertThat(retry.maxAttempts()).isZero();
    }
}
