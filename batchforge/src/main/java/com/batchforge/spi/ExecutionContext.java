package com.batchforge.spi;

import java.util.Map;
import java.util.UUID;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public record ExecutionContext(
    UUID jobExecutionId,
    UUID stepExecutionId,
    String stepName,
    String partitionId,
    Map<String, Object> jobParameters,
    Map<String, Object> stepProperties
) {
    private static final Pattern PARAM_PATTERN = Pattern.compile(":(\\w+)");

    public String resolveTemplate(String template) {
        Matcher m = PARAM_PATTERN.matcher(template);
        StringBuilder sb = new StringBuilder();
        while (m.find()) {
            String key = m.group(1);
            Object value = jobParameters.get(key);
            m.appendReplacement(sb, value != null ? Matcher.quoteReplacement(value.toString()) : Matcher.quoteReplacement(m.group(0)));
        }
        m.appendTail(sb);
        return sb.toString();
    }
}
