package com.batchforge.engine.core;

import java.util.List;

public record StepDefinition(
    String name,
    ReaderConfig reader,
    String processorClass,
    WriterConfig writer,
    int chunkSize,
    int partitions,
    List<String> dependsOn,
    RetryConfig retry,
    boolean skipOnFail
) {
    public StepDefinition {
        if (chunkSize <= 0) chunkSize = 100;
        if (partitions <= 0) partitions = 1;
        if (dependsOn == null) dependsOn = List.of();
        if (retry == null) retry = RetryConfig.none();
    }
}
