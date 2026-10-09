package com.batchforge.engine.repository;

import com.batchforge.engine.core.*;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;

import javax.sql.DataSource;
import java.sql.*;
import java.time.Instant;
import java.util.*;

@ApplicationScoped
public class StepExecutionRepository {

    private static final String COLS = "id, job_execution_id, step_name, status, partition_id, chunks_processed, "
        + "chunks_total, items_read, items_written, items_failed, started_at, finished_at, error_message, attempt_count";

    @Inject DataSource dataSource;

    @Transactional
    public StepExecution save(StepExecution s) {
        String sql = "INSERT INTO step_executions (" + COLS + ") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)";
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            UUID id = s.id() != null ? s.id() : UUID.randomUUID();
            ps.setObject(1, id);
            ps.setObject(2, s.jobExecutionId());
            ps.setString(3, s.stepName());
            ps.setString(4, s.status().toDbValue());
            ps.setString(5, s.partitionId());
            ps.setInt(6, s.chunksProcessed());
            ps.setObject(7, s.chunksTotal(), Types.INTEGER);
            ps.setLong(8, s.itemsRead());
            ps.setLong(9, s.itemsWritten());
            ps.setLong(10, s.itemsFailed());
            ps.setTimestamp(11, ts(s.startedAt()));
            ps.setTimestamp(12, ts(s.finishedAt()));
            ps.setString(13, s.errorMessage());
            ps.setInt(14, attemptOf(s.status()));
            ps.executeUpdate();
            return new StepExecution(id, s.jobExecutionId(), s.stepName(), s.status(), s.partitionId(),
                s.chunksProcessed(), s.chunksTotal(), s.itemsRead(), s.itemsWritten(), s.itemsFailed(),
                s.startedAt(), s.finishedAt(), s.errorMessage());
        } catch (SQLException e) {
            throw new RepositoryException("Failed to save step execution", e);
        }
    }

    public List<StepExecution> findByJobExecutionId(UUID jobExecutionId) {
        String sql = "SELECT " + COLS + " FROM step_executions WHERE job_execution_id = ? ORDER BY step_name, partition_id";
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setObject(1, jobExecutionId);
            try (ResultSet rs = ps.executeQuery()) {
                List<StepExecution> out = new ArrayList<>();
                while (rs.next()) out.add(map(rs));
                return out;
            }
        } catch (SQLException e) {
            throw new RepositoryException("Failed to list step executions", e);
        }
    }

    @Transactional
    public void updateStatus(UUID id, StepStatus status) {
        String message = switch (status) {
            case StepStatus.StepFailed f -> f.message();
            case StepStatus.Skipped sk -> sk.reason();
            default -> null;
        };
        String v = status.toDbValue();
        String sql = """
            UPDATE step_executions SET status = ?, error_message = COALESCE(?, error_message),
              started_at = CASE WHEN ? = 'RUNNING' AND started_at IS NULL THEN now() ELSE started_at END,
              finished_at = CASE WHEN ? IN ('COMPLETED','FAILED','SKIPPED') THEN now() ELSE finished_at END,
              attempt_count = ?
            WHERE id = ?
            """;
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setString(1, v);
            ps.setString(2, message);
            ps.setString(3, v);
            ps.setString(4, v);
            ps.setInt(5, attemptOf(status));
            ps.setObject(6, id);
            ps.executeUpdate();
        } catch (SQLException e) {
            throw new RepositoryException("Failed to update step status " + id, e);
        }
    }

    @Transactional
    public void updateProgress(UUID id, int chunksProcessed, long itemsRead, long itemsWritten, long itemsFailed) {
        String sql = "UPDATE step_executions SET chunks_processed = ?, items_read = ?, items_written = ?, items_failed = ? WHERE id = ?";
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setInt(1, chunksProcessed);
            ps.setLong(2, itemsRead);
            ps.setLong(3, itemsWritten);
            ps.setLong(4, itemsFailed);
            ps.setObject(5, id);
            ps.executeUpdate();
        } catch (SQLException e) {
            throw new RepositoryException("Failed to update step progress " + id, e);
        }
    }

    private StepExecution map(ResultSet rs) throws SQLException {
        String msg = rs.getString(13);
        Timestamp s = rs.getTimestamp(11), f = rs.getTimestamp(12);
        return new StepExecution(rs.getObject(1, UUID.class), rs.getObject(2, UUID.class), rs.getString(3),
            StepStatus.fromDbValue(rs.getString(4), msg, rs.getInt(14)), rs.getString(5), rs.getInt(6),
            rs.getObject(7, Integer.class), rs.getLong(8), rs.getLong(9), rs.getLong(10),
            s == null ? null : s.toInstant(), f == null ? null : f.toInstant(), msg);
    }

    private static int attemptOf(StepStatus status) {
        return status instanceof StepStatus.Retrying r ? r.attempt() : 0;
    }

    private static Timestamp ts(Instant i) {
        return i == null ? null : Timestamp.from(i);
    }
}
