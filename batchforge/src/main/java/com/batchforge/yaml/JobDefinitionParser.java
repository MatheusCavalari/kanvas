package com.batchforge.yaml;

import com.batchforge.engine.core.*;
import jakarta.enterprise.context.ApplicationScoped;
import org.yaml.snakeyaml.Yaml;

import java.io.IOException;
import java.nio.file.*;
import java.time.Duration;
import java.util.*;
import java.util.regex.*;

@ApplicationScoped
public class JobDefinitionParser {

    private static final Pattern DURATION_PATTERN = Pattern.compile("(\\d+)(ms|s|m|h)");

    public JobDefinition parseFile(Path file) throws IOException {
        return parse(Files.readString(file));
    }

    @SuppressWarnings("unchecked")
    public JobDefinition parse(String yamlContent) {
        Yaml yaml = new Yaml();
        Map<String, Object> root = yaml.load(yamlContent);

        String name = requireString(root, "name");
        String description = (String) root.getOrDefault("description", "");
        String schedule = (String) root.get("schedule");

        Map<String, ParameterDefinition> parameters = parseParameters(
            (Map<String, Object>) root.getOrDefault("parameters", Map.of()));

        Map<String, Object> stepsRaw = (Map<String, Object>) root.get("steps");
        if (stepsRaw == null || stepsRaw.isEmpty()) {
            throw new JobValidationException("Job '" + name + "' must define at least one step");
        }

        Map<String, StepDefinition> steps = new LinkedHashMap<>();
        for (var entry : stepsRaw.entrySet()) {
            steps.put(entry.getKey(), parseStep(entry.getKey(), (Map<String, Object>) entry.getValue()));
        }

        return new JobDefinition(name, description, schedule, parameters, steps);
    }

    @SuppressWarnings("unchecked")
    private Map<String, ParameterDefinition> parseParameters(Map<String, Object> raw) {
        Map<String, ParameterDefinition> params = new LinkedHashMap<>();
        for (var entry : raw.entrySet()) {
            Map<String, Object> def = (Map<String, Object>) entry.getValue();
            params.put(entry.getKey(), new ParameterDefinition(
                (String) def.getOrDefault("type", "string"),
                Boolean.TRUE.equals(def.get("required")),
                def.get("default") != null ? def.get("default").toString() : null
            ));
        }
        return params;
    }

    @SuppressWarnings("unchecked")
    private StepDefinition parseStep(String name, Map<String, Object> raw) {
        Map<String, Object> readerRaw = (Map<String, Object>) raw.get("reader");
        if (readerRaw == null) throw new JobValidationException("Step '" + name + "' missing reader");
        ReaderConfig reader = new ReaderConfig(
            requireString(readerRaw, "type"),
            extractProperties(readerRaw)
        );

        String processorClass = (String) raw.get("processor");
        if (processorClass == null) throw new JobValidationException("Step '" + name + "' missing processor");

        Map<String, Object> writerRaw = (Map<String, Object>) raw.get("writer");
        if (writerRaw == null) throw new JobValidationException("Step '" + name + "' missing writer");
        WriterConfig writer = new WriterConfig(
            requireString(writerRaw, "type"),
            extractProperties(writerRaw)
        );

        int chunkSize = ((Number) raw.getOrDefault("chunk-size", 100)).intValue();
        int partitions = ((Number) raw.getOrDefault("partitions", 1)).intValue();
        List<String> dependsOn = (List<String>) raw.getOrDefault("depends-on", List.of());
        boolean skipOnFail = Boolean.TRUE.equals(raw.get("skip-on-fail"));

        RetryConfig retry = raw.containsKey("retry")
            ? parseRetryConfig((Map<String, Object>) raw.get("retry"))
            : RetryConfig.none();

        return new StepDefinition(name, reader, processorClass, writer, chunkSize, partitions, dependsOn, retry, skipOnFail);
    }

    @SuppressWarnings("unchecked")
    private RetryConfig parseRetryConfig(Map<String, Object> raw) {
        int maxAttempts = ((Number) raw.getOrDefault("max-attempts", 3)).intValue();
        String backoff = (String) raw.getOrDefault("backoff", "exponential");
        Duration baseDelay = parseDuration((String) raw.getOrDefault("base-delay", "1s"));
        Duration maxDelay = parseDuration((String) raw.getOrDefault("max-delay", "30s"));
        List<String> retryable = (List<String>) raw.getOrDefault("retryable-exceptions", List.of());
        return new RetryConfig(maxAttempts, backoff, baseDelay, maxDelay, retryable);
    }

    private Duration parseDuration(String value) {
        Matcher m = DURATION_PATTERN.matcher(value);
        if (!m.matches()) throw new JobValidationException("Invalid duration: " + value);
        long amount = Long.parseLong(m.group(1));
        return switch (m.group(2)) {
            case "ms" -> Duration.ofMillis(amount);
            case "s" -> Duration.ofSeconds(amount);
            case "m" -> Duration.ofMinutes(amount);
            case "h" -> Duration.ofHours(amount);
            default -> throw new JobValidationException("Unknown duration unit: " + m.group(2));
        };
    }

    private Map<String, String> extractProperties(Map<String, Object> raw) {
        Map<String, String> props = new LinkedHashMap<>();
        for (var entry : raw.entrySet()) {
            if (!"type".equals(entry.getKey()) && entry.getValue() != null) {
                props.put(entry.getKey(), entry.getValue().toString());
            }
        }
        return props;
    }

    private String requireString(Map<String, Object> map, String key) {
        Object val = map.get(key);
        if (val == null) throw new JobValidationException("Missing required field: " + key);
        return val.toString();
    }
}
