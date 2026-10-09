package com.batchforge.engine.dag;

import com.batchforge.engine.executor.ChunkResult;

public record StepExecutionResult(String stepName, boolean success, ChunkResult result, String errorMessage) {}
