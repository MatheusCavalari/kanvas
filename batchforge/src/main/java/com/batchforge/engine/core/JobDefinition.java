package com.batchforge.engine.core;

import java.util.Map;

public record JobDefinition(
    String name,
    String description,
    String schedule,
    Map<String, ParameterDefinition> parameters,
    Map<String, StepDefinition> steps
) {
    public JobDefinition {
        if (parameters == null) parameters = Map.of();
        if (steps == null || steps.isEmpty()) {
            throw new IllegalArgumentException("Job must have at least one step");
        }
    }
}
