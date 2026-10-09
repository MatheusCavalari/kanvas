package com.batchforge.engine.core;

public sealed interface ExecutionStatus {
    record Pending() implements ExecutionStatus {}
    record Running() implements ExecutionStatus {}
    record Completed() implements ExecutionStatus {}
    record Failed(String message) implements ExecutionStatus {}
    record Restarting() implements ExecutionStatus {}
    record Abandoned(String message) implements ExecutionStatus {}

    default String toDbValue() {
        return switch (this) {
            case Pending _ -> "PENDING";
            case Running _ -> "RUNNING";
            case Completed _ -> "COMPLETED";
            case Failed _ -> "FAILED";
            case Restarting _ -> "RESTARTING";
            case Abandoned _ -> "ABANDONED";
        };
    }

    static ExecutionStatus fromDbValue(String value, String message) {
        return switch (value) {
            case "PENDING" -> new Pending();
            case "RUNNING" -> new Running();
            case "COMPLETED" -> new Completed();
            case "FAILED" -> new Failed(message);
            case "RESTARTING" -> new Restarting();
            case "ABANDONED" -> new Abandoned(message);
            default -> throw new IllegalArgumentException("Unknown execution status: " + value);
        };
    }
}
