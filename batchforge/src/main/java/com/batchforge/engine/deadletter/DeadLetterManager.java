package com.batchforge.engine.deadletter;

import com.batchforge.engine.core.DeadLetterItem;
import com.batchforge.engine.repository.DeadLetterRepository;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.util.UUID;

@ApplicationScoped
public class DeadLetterManager {

    @Inject DeadLetterRepository repository;
    @Inject ObjectMapper objectMapper;

    public void send(UUID stepExecutionId, String stepName, Object item, Exception error, int attempt) {
        String payload;
        try {
            payload = objectMapper.writeValueAsString(item);
        } catch (Exception e) {
            payload = item.toString();
        }
        StringWriter sw = new StringWriter();
        error.printStackTrace(new PrintWriter(sw));

        repository.save(DeadLetterItem.create(
            stepExecutionId, stepName, payload, error.getMessage(), sw.toString(), attempt
        ));
    }

    public void retryItem(UUID itemId) {
        repository.updateStatus(itemId, "RETRIED");
    }

    public void discardItem(UUID itemId) {
        repository.updateStatus(itemId, "DISCARDED");
    }
}
