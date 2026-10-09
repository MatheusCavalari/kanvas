package com.batchforge.engine.checkpoint;

import com.batchforge.engine.core.*;
import com.batchforge.engine.repository.*;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Test;

import java.util.*;

import static org.assertj.core.api.Assertions.*;

@QuarkusTest
@QuarkusTestResource(TestcontainersConfig.class)
class CheckpointManagerIntegrationTest {

    @Inject CheckpointManager checkpointManager;
    @Inject JobDefinitionRepository jobDefRepo;
    @Inject JobExecutionRepository jobExecRepo;
    @Inject StepExecutionRepository stepExecRepo;

    private UUID setupStepExecution(String jobName) {
        var job = new JobDefinition(jobName, "test", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));
        jobDefRepo.save(job, "{}", "h");
        var exec = jobExecRepo.save(JobExecution.create(jobName, Map.of()));
        return stepExecRepo.save(StepExecution.create(exec.id(), "s1", "main")).id();
    }

    @Test
    void saveAndRetrieveCheckpoint() {
        UUID stepExecId = setupStepExecution("cp-test-1");

        checkpointManager.save(stepExecId, "main", 500, Map.of());
        var offset = checkpointManager.getLastOffset(stepExecId, "main");
        assertThat(offset).hasValue(500);
    }

    @Test
    void checkpointUpsertsOnConflict() {
        UUID stepExecId = setupStepExecution("cp-test-2");

        checkpointManager.save(stepExecId, "main", 100, Map.of());
        checkpointManager.save(stepExecId, "main", 200, Map.of());
        var offset = checkpointManager.getLastOffset(stepExecId, "main");
        assertThat(offset).hasValue(200);
    }

    @Test
    void returnsEmptyForNoCheckpoint() {
        UUID stepExecId = setupStepExecution("cp-test-3");
        var offset = checkpointManager.getLastOffset(stepExecId, "main");
        assertThat(offset).isEmpty();
    }
}
