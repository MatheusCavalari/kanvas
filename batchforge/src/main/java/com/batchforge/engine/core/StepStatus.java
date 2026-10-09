package com.batchforge.engine.core;

public sealed interface StepStatus {
    record Waiting() implements StepStatus {}
    record Ready() implements StepStatus {}
    record StepRunning() implements StepStatus {}
    record StepCompleted() implements StepStatus {}
    record StepFailed(String message) implements StepStatus {}
    record Retrying(int attempt) implements StepStatus {}
    record Skipped(String reason) implements StepStatus {}

    default String toDbValue() {
        return switch (this) {
            case Waiting _ -> "WAITING";
            case Ready _ -> "READY";
            case StepRunning _ -> "RUNNING";
            case StepCompleted _ -> "COMPLETED";
            case StepFailed _ -> "FAILED";
            case Retrying _ -> "RETRYING";
            case Skipped _ -> "SKIPPED";
        };
    }

    static StepStatus fromDbValue(String value, String message) {
        return switch (value) {
            case "WAITING" -> new Waiting();
            case "READY" -> new Ready();
            case "RUNNING" -> new StepRunning();
            case "COMPLETED" -> new StepCompleted();
            case "FAILED" -> new StepFailed(message);
            case "RETRYING" -> new Retrying(0);
            case "SKIPPED" -> new Skipped(message);
            default -> throw new IllegalArgumentException("Unknown step status: " + value);
        };
    }
}
