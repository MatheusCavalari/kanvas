package com.batchforge.engine;

import com.batchforge.engine.repository.JobExecutionRepository;
import com.batchforge.engine.repository.TestcontainersConfig;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;

@QuarkusTest
@QuarkusTestResource(TestcontainersConfig.class)
class EngineIntegrationTest {

    @Inject BatchForgeEngine engine;
    @Inject JobExecutionRepository jobExecRepo;

    @TempDir Path tempDir;

    @Test
    void executesSimpleJobEndToEnd() throws Exception {
        Files.writeString(tempDir.resolve("input.csv"), "id,name\n1,Alice\n2,Bob\n3,Carol\n");

        Path jobFile = tempDir.resolve("simple-test.yml");
        Files.writeString(jobFile, """
            name: e2e-test
            description: integration test
            steps:
              read-and-write:
                reader:
                  type: csv
                  path: "%s/input.csv"
                processor: com.batchforge.engine.TestPassthroughProcessor
                writer:
                  type: csv
                  path: "%s/output.csv"
                chunk-size: 2
            """.formatted(tempDir, tempDir));

        engine.loadJobFile(jobFile);
        UUID execId = engine.executeJob("e2e-test", Map.of());

        Thread.sleep(2000);

        var exec = jobExecRepo.findById(execId);
        assertThat(exec).isPresent();
        assertThat(exec.get().status().toDbValue()).isEqualTo("COMPLETED");

        String output = Files.readString(tempDir.resolve("output.csv"));
        assertThat(output).contains("Alice").contains("Bob").contains("Carol");
    }
}
