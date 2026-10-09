package com.batchforge.engine.repository;

import com.batchforge.engine.core.DeadLetterItem;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;

import javax.sql.DataSource;
import java.sql.*;
import java.time.Instant;
import java.util.*;

@ApplicationScoped
public class DeadLetterRepository {

    private static final String COLS =
        "id, step_execution_id, step_name, item_payload::text, error_message, stack_trace, attempt_count, status, failed_at";

    @Inject DataSource dataSource;

    @Transactional
    public void save(DeadLetterItem d) {
        String sql = """
            INSERT INTO dead_letter_items (id, step_execution_id, step_name, item_payload, error_message,
              stack_trace, attempt_count, status, failed_at)
            VALUES (?, ?, ?, ?::jsonb, ?, ?, ?, ?, ?)
            """;
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setObject(1, d.id() != null ? d.id() : UUID.randomUUID());
            ps.setObject(2, d.stepExecutionId());
            ps.setString(3, d.stepName());
            ps.setString(4, d.itemPayload());
            ps.setString(5, d.errorMessage());
            ps.setString(6, d.stackTrace());
            ps.setInt(7, d.attemptCount());
            ps.setString(8, d.status() != null ? d.status() : "PENDING");
            ps.setTimestamp(9, Timestamp.from(d.failedAt() != null ? d.failedAt() : Instant.now()));
            ps.executeUpdate();
        } catch (SQLException e) {
            throw new RepositoryException("Failed to save dead letter item", e);
        }
    }

    public List<DeadLetterItem> findByStepExecution(UUID stepExecutionId) {
        return query("SELECT " + COLS + " FROM dead_letter_items WHERE step_execution_id = ? ORDER BY failed_at",
            ps -> ps.setObject(1, stepExecutionId));
    }

    public List<DeadLetterItem> findPending(int limit) {
        return query("SELECT " + COLS + " FROM dead_letter_items WHERE status = 'PENDING' ORDER BY failed_at LIMIT ?",
            ps -> ps.setInt(1, limit));
    }

    @Transactional
    public void updateStatus(UUID itemId, String status) {
        try (Connection c = dataSource.getConnection();
             PreparedStatement ps = c.prepareStatement("UPDATE dead_letter_items SET status = ? WHERE id = ?")) {
            ps.setString(1, status);
            ps.setObject(2, itemId);
            ps.executeUpdate();
        } catch (SQLException e) {
            throw new RepositoryException("Failed to update dead letter status " + itemId, e);
        }
    }

    private interface Binder { void bind(PreparedStatement ps) throws SQLException; }

    private List<DeadLetterItem> query(String sql, Binder binder) {
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            binder.bind(ps);
            try (ResultSet rs = ps.executeQuery()) {
                List<DeadLetterItem> out = new ArrayList<>();
                while (rs.next()) {
                    out.add(new DeadLetterItem(rs.getObject(1, UUID.class), rs.getObject(2, UUID.class),
                        rs.getString(3), rs.getString(4), rs.getString(5), rs.getString(6),
                        rs.getInt(7), rs.getString(8), rs.getTimestamp(9).toInstant()));
                }
                return out;
            }
        } catch (SQLException e) {
            throw new RepositoryException("Failed to query dead letter items", e);
        }
    }
}
