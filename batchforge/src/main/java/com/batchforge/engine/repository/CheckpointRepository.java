package com.batchforge.engine.repository;

import com.batchforge.engine.core.Checkpoint;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;

import javax.sql.DataSource;
import java.sql.*;
import java.time.Instant;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@ApplicationScoped
public class CheckpointRepository {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Inject DataSource dataSource;

    @Transactional
    public void save(Checkpoint cp) {
        String sql = """
            INSERT INTO checkpoints (step_execution_id, partition_id, reader_offset, metadata, saved_at)
            VALUES (?, ?, ?, ?::jsonb, ?)
            ON CONFLICT (step_execution_id, partition_id) DO UPDATE SET
              reader_offset = EXCLUDED.reader_offset, metadata = EXCLUDED.metadata, saved_at = EXCLUDED.saved_at
            """;
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setObject(1, cp.stepExecutionId());
            ps.setString(2, cp.partitionId());
            ps.setLong(3, cp.offset());
            ps.setString(4, MAPPER.writeValueAsString(cp.metadata() == null ? Map.of() : cp.metadata()));
            ps.setTimestamp(5, Timestamp.from(cp.savedAt() != null ? cp.savedAt() : Instant.now()));
            ps.executeUpdate();
        } catch (SQLException | JsonProcessingException e) {
            throw new RepositoryException("Failed to save checkpoint", e);
        }
    }

    public Optional<Checkpoint> findLatest(UUID stepExecutionId, String partitionId) {
        String sql = "SELECT step_execution_id, partition_id, reader_offset, metadata::text, saved_at "
            + "FROM checkpoints WHERE step_execution_id = ? AND partition_id = ?";
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setObject(1, stepExecutionId);
            ps.setString(2, partitionId);
            try (ResultSet rs = ps.executeQuery()) {
                if (!rs.next()) return Optional.empty();
                String md = rs.getString(4);
                Map<String, Object> metadata = md == null ? Map.of()
                    : MAPPER.readValue(md, new TypeReference<Map<String, Object>>() {});
                return Optional.of(new Checkpoint(rs.getObject(1, UUID.class), rs.getString(2),
                    rs.getLong(3), metadata, rs.getTimestamp(5).toInstant()));
            }
        } catch (SQLException | JsonProcessingException e) {
            throw new RepositoryException("Failed to find checkpoint", e);
        }
    }
}
