package com.batchforge.engine.checkpoint;

import com.batchforge.engine.core.Checkpoint;
import com.batchforge.engine.repository.CheckpointRepository;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.time.Instant;
import java.util.Map;
import java.util.OptionalLong;
import java.util.UUID;

@ApplicationScoped
public class CheckpointManager {

    @Inject CheckpointRepository repository;

    public void save(UUID stepExecutionId, String partitionId, long offset, Map<String, Object> metadata) {
        repository.save(new Checkpoint(stepExecutionId, partitionId, offset, metadata, Instant.now()));
    }

    public OptionalLong getLastOffset(UUID stepExecutionId, String partitionId) {
        return repository.findLatest(stepExecutionId, partitionId)
            .map(cp -> OptionalLong.of(cp.offset()))
            .orElse(OptionalLong.empty());
    }
}
