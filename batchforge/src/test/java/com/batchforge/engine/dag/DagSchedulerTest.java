package com.batchforge.engine.dag;

import com.batchforge.engine.core.*;
import org.junit.jupiter.api.Test;

import java.util.*;

import static org.assertj.core.api.Assertions.*;

class DagSchedulerTest {

    private static StepDefinition step(String name, List<String> deps) {
        return new StepDefinition(name,
            new ReaderConfig("test", Map.of()), "com.Proc",
            new WriterConfig("test", Map.of()), 100, 1, deps, RetryConfig.none(), false);
    }

    @Test
    void executesStepsInTopologicalOrder() {
        var steps = new LinkedHashMap<String, StepDefinition>();
        steps.put("A", step("A", List.of()));
        steps.put("B", step("B", List.of("A")));
        steps.put("C", step("C", List.of("B")));
        var jobDef = new JobDefinition("test-job", "test", null, Map.of(), steps);

        var levels = new DagValidator().topologicalSort(jobDef);

        assertThat(levels).hasSize(3);
        assertThat(levels.get(0)).containsExactly("A");
        assertThat(levels.get(1)).containsExactly("B");
        assertThat(levels.get(2)).containsExactly("C");
    }

    @Test
    void parallelStepsRunInSameLevel() {
        var steps = new LinkedHashMap<String, StepDefinition>();
        steps.put("A", step("A", List.of()));
        steps.put("B", step("B", List.of()));
        steps.put("C", step("C", List.of("A", "B")));
        var jobDef = new JobDefinition("test-job", "test", null, Map.of(), steps);

        var levels = new DagValidator().topologicalSort(jobDef);

        assertThat(levels).hasSize(2);
        assertThat(levels.get(0)).containsExactlyInAnyOrder("A", "B");
        assertThat(levels.get(1)).containsExactly("C");
    }
}
