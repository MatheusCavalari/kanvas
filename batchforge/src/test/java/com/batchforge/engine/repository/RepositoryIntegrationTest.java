package com.batchforge.engine.repository;

import com.batchforge.engine.core.*;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

@QuarkusTest
@QuarkusTestResource(TestcontainersConfig.class)
class RepositoryIntegrationTest {

    @Inject JobDefinitionRepository jobDefRepo;
    @Inject JobExecutionRepository jobExecRepo;
    @Inject StepExecutionRepository stepExecRepo;
    @Inject CheckpointRepository checkpointRepo;
    @Inject DeadLetterRepository deadLetterRepo;

    private static JobDefinition job(String name) {
        return new JobDefinition(name, "test", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));
    }

    @Test
    void jobDefinitionCrud() {
        jobDefRepo.save(job("test-job"), "{\"steps\":[\"s1\"]}", "abc123");
        var found = jobDefRepo.findByName("test-job");
        assertThat(found).isPresent();
        assertThat(found.get().name()).isEqualTo("test-job");

        jobDefRepo.save(job("test-job"), "{\"steps\":[\"s1\"]}", "abc456");
        assertThat(jobDefRepo.findAll()).extracting(JobDefinition::name).contains("test-job");

        jobDefRepo.delete("test-job");
        assertThat(jobDefRepo.findByName("test-job")).isEmpty();
    }

    @Test
    void executionLifecycle() {
        jobDefRepo.save(job("exec-test-job"), "{}", "hash1");

        var exec = JobExecution.create("exec-test-job", Map.of("key", "val"));
        exec = jobExecRepo.save(exec);
        assertThat(exec.id()).isNotNull();

        jobExecRepo.updateStatus(exec.id(), new ExecutionStatus.Running());
        var updated = jobExecRepo.findById(exec.id());
        assertThat(updated).isPresent();
        assertThat(updated.get().status()).isInstanceOf(ExecutionStatus.Running.class);
        assertThat(updated.get().parameters()).containsEntry("key", "val");
        assertThat(jobExecRepo.findByJobName("exec-test-job", 5)).hasSize(1);
    }

    @Test
    void stepExecutionWithCheckpoint() {
        jobDefRepo.save(job("step-test-job"), "{}", "hash2");

        var exec = jobExecRepo.save(JobExecution.create("step-test-job", Map.of()));
        var stepExec = stepExecRepo.save(StepExecution.create(exec.id(), "s1", "main"));

        stepExecRepo.updateProgress(stepExec.id(), 2, 200, 190, 10);
        stepExecRepo.updateStatus(stepExec.id(), new StepStatus.StepRunning());
        var steps = stepExecRepo.findByJobExecutionId(exec.id());
        assertThat(steps).hasSize(1);
        assertThat(steps.getFirst().itemsRead()).isEqualTo(200);

        checkpointRepo.save(new Checkpoint(stepExec.id(), "main", 400, Map.of(), Instant.now()));
        checkpointRepo.save(new Checkpoint(stepExec.id(), "main", 500, Map.of(), Instant.now()));
        var cp = checkpointRepo.findLatest(stepExec.id(), "main");
        assertThat(cp).isPresent();
        assertThat(cp.get().offset()).isEqualTo(500);
    }

    @Test
    void deadLetterItemPersistence() {
        jobDefRepo.save(job("dlq-test-job"), "{}", "hash3");

        var exec = jobExecRepo.save(JobExecution.create("dlq-test-job", Map.of()));
        var stepExec = stepExecRepo.save(StepExecution.create(exec.id(), "s1", "main"));

        var dlq = DeadLetterItem.create(stepExec.id(), "s1", "{\"id\":1}", "NPE", "stack...", 3);
        deadLetterRepo.save(dlq);

        var items = deadLetterRepo.findByStepExecution(stepExec.id());
        assertThat(items).hasSize(1);
        assertThat(items.getFirst().errorMessage()).isEqualTo("NPE");

        var pending = deadLetterRepo.findPending(10);
        assertThat(pending).isNotEmpty();

        deadLetterRepo.updateStatus(dlq.id(), "RESOLVED");
        assertThat(deadLetterRepo.findByStepExecution(stepExec.id()).getFirst().status()).isEqualTo("RESOLVED");
    }
}
