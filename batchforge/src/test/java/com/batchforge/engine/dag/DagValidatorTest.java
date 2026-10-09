package com.batchforge.engine.dag;

import com.batchforge.engine.core.*;
import com.batchforge.yaml.JobDefinitionParser;
import org.junit.jupiter.api.Test;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.List;

import static org.assertj.core.api.Assertions.*;

class DagValidatorTest {

    private final JobDefinitionParser parser = new JobDefinitionParser();
    private final DagValidator validator = new DagValidator();

    private String loadYaml(String resource) throws Exception {
        try (InputStream is = getClass().getResourceAsStream(resource)) {
            return new String(is.readAllBytes(), StandardCharsets.UTF_8);
        }
    }

    @Test
    void validJobPassesValidation() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        List<String> errors = validator.validate(job);
        assertThat(errors).isEmpty();
    }

    @Test
    void detectsCyclicDependency() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/invalid-cycle.yml"));
        List<String> errors = validator.validate(job);
        assertThat(errors).anyMatch(e -> e.contains("cycle"));
    }

    @Test
    void detectsMissingDependency() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/invalid-missing-dep.yml"));
        List<String> errors = validator.validate(job);
        assertThat(errors).anyMatch(e -> e.contains("step-x"));
    }

    @Test
    void topologicalSortReturnsLevels() throws Exception {
        JobDefinition job = parser.parse(loadYaml("/jobs/valid-reconciliation.yml"));
        List<List<String>> levels = validator.topologicalSort(job);

        assertThat(levels.get(0)).containsExactlyInAnyOrder("load-transactions", "load-statements");
        assertThat(levels.get(1)).containsExactly("match");
        assertThat(levels.get(2)).containsExactly("generate-report");
    }
}
