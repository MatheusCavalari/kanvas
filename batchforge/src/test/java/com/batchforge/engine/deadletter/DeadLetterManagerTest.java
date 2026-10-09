package com.batchforge.engine.deadletter;

import com.batchforge.engine.core.DeadLetterItem;
import com.batchforge.engine.repository.DeadLetterRepository;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;

class DeadLetterManagerTest {

    /** Hand-written stub: records calls instead of touching a database. */
    private static class RecordingRepository extends DeadLetterRepository {
        final List<DeadLetterItem> saved = new ArrayList<>();
        final List<String> statusUpdates = new ArrayList<>();
        UUID lastUpdatedId;

        @Override
        public void save(DeadLetterItem d) {
            saved.add(d);
        }

        @Override
        public void updateStatus(UUID itemId, String status) {
            lastUpdatedId = itemId;
            statusUpdates.add(status);
        }
    }

    private RecordingRepository repository;
    private DeadLetterManager manager;

    @BeforeEach
    void setUp() {
        repository = new RecordingRepository();
        manager = new DeadLetterManager();
        manager.repository = repository;
        manager.objectMapper = new ObjectMapper();
    }

    @Test
    void sendSerializesItemAndSavesPendingEntry() {
        UUID stepExecutionId = UUID.randomUUID();
        var error = new IllegalStateException("boom");

        manager.send(stepExecutionId, "transform", Map.of("id", 42), error, 3);

        assertThat(repository.saved).hasSize(1);
        DeadLetterItem saved = repository.saved.get(0);
        assertThat(saved.stepExecutionId()).isEqualTo(stepExecutionId);
        assertThat(saved.stepName()).isEqualTo("transform");
        assertThat(saved.itemPayload()).isEqualTo("{\"id\":42}");
        assertThat(saved.errorMessage()).isEqualTo("boom");
        assertThat(saved.stackTrace()).contains("IllegalStateException");
        assertThat(saved.attemptCount()).isEqualTo(3);
        assertThat(saved.status()).isEqualTo("PENDING");
    }

    @Test
    void retryItemMarksRetried() {
        UUID id = UUID.randomUUID();
        manager.retryItem(id);
        assertThat(repository.lastUpdatedId).isEqualTo(id);
        assertThat(repository.statusUpdates).containsExactly("RETRIED");
    }

    @Test
    void discardItemMarksDiscarded() {
        UUID id = UUID.randomUUID();
        manager.discardItem(id);
        assertThat(repository.lastUpdatedId).isEqualTo(id);
        assertThat(repository.statusUpdates).containsExactly("DISCARDED");
    }
}
