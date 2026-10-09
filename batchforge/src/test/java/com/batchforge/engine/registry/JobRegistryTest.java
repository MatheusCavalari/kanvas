package com.batchforge.engine.registry;

import com.batchforge.engine.core.*;
import com.batchforge.engine.dag.DagValidator;
import com.batchforge.engine.repository.JobDefinitionRepository;
import com.batchforge.yaml.JobDefinitionParser;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.*;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class JobRegistryTest {

    @TempDir Path tempDir;

    static class FakeRepo extends JobDefinitionRepository {
        final List<String> saved = new ArrayList<>();
        @Override
        public void save(JobDefinition job, String dagJson, String yamlHash) {
            saved.add(job.name());
        }
    }

    private JobRegistry registry(FakeRepo repo) {
        return new JobRegistry(new JobDefinitionParser(), new DagValidator(), repo);
    }

    @Test
    void registersAndRetrievesJob() {
        var repo = new FakeRepo();
        var registry = registry(repo);

        var job = new JobDefinition("test-job", "desc", null, Map.of(),
            Map.of("s1", new StepDefinition("s1",
                new ReaderConfig("jdbc", Map.of()), "com.Proc",
                new WriterConfig("jdbc", Map.of()), 100, 1, List.of(), RetryConfig.none(), false)));

        registry.register(job);
        assertThat(registry.getJob("test-job")).isPresent();
        assertThat(registry.listJobs()).hasSize(1);
        assertThat(repo.saved).containsExactly("test-job");
    }

    @Test
    void loadsFromDirectory() throws Exception {
        Files.writeString(tempDir.resolve("test.yml"), """
            name: dir-job
            description: loaded from dir
            steps:
              step1:
                reader: { type: csv, path: /data/f.csv }
                processor: com.Proc
                writer: { type: jdbc, table: t }
            """);

        var registry = registry(new FakeRepo());
        registry.loadFromDirectory(tempDir);
        assertThat(registry.getJob("dir-job")).isPresent();
    }

    @Test
    void rejectsInvalidJob() throws Exception {
        Files.writeString(tempDir.resolve("bad.yml"), """
            name: bad-job
            description: has cycle
            steps:
              a:
                depends-on: [b]
                reader: { type: jdbc, query: "SELECT 1" }
                processor: com.P
                writer: { type: jdbc, table: t }
              b:
                depends-on: [a]
                reader: { type: jdbc, query: "SELECT 1" }
                processor: com.P
                writer: { type: jdbc, table: t }
            """);

        var repo = new FakeRepo();
        var registry = registry(repo);
        registry.loadFromDirectory(tempDir);
        assertThat(registry.getJob("bad-job")).isEmpty();
        assertThat(repo.saved).isEmpty();
    }
}
