package com.batchforge.engine.repository;

import com.batchforge.engine.core.*;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;

import javax.sql.DataSource;
import java.sql.*;
import java.time.Instant;
import java.util.*;

@ApplicationScoped
public class JobExecutionRepository {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final String COLS =
        "id, job_name, status, parameters::text, started_at, finished_at, restart_count, error_message";

    @Inject DataSource dataSource;

    @Transactional
    public JobExecution save(JobExecution e) {
        String sql = """
            INSERT INTO job_executions (id, job_name, status, parameters, started_at, finished_at, restart_count, error_message)
            VALUES (?, ?, ?, ?::jsonb, ?, ?, ?, ?)
            """;
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            UUID id = e.id() != null ? e.id() : UUID.randomUUID();
            ps.setObject(1, id);
            ps.setString(2, e.jobName());
            ps.setString(3, e.status().toDbValue());
            ps.setString(4, MAPPER.writeValueAsString(e.parameters() == null ? Map.of() : e.parameters()));
            ps.setTimestamp(5, ts(e.startedAt()));
            ps.setTimestamp(6, ts(e.finishedAt()));
            ps.setInt(7, e.restartCount());
            ps.setString(8, e.errorMessage());
            ps.executeUpdate();
            return new JobExecution(id, e.jobName(), e.status(), e.parameters(), e.startedAt(),
                e.finishedAt(), e.metrics(), e.restartCount(), e.errorMessage());
        } catch (SQLException | JsonProcessingException ex) {
            throw new RepositoryException("Failed to save job execution", ex);
        }
    }

    public Optional<JobExecution> findById(UUID id) {
        try (Connection c = dataSource.getConnection();
             PreparedStatement ps = c.prepareStatement("SELECT " + COLS + " FROM job_executions WHERE id = ?")) {
            ps.setObject(1, id);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() ? Optional.of(map(rs)) : Optional.empty();
            }
        } catch (SQLException | JsonProcessingException ex) {
            throw new RepositoryException("Failed to find job execution " + id, ex);
        }
    }

    @Transactional
    public void updateStatus(UUID id, ExecutionStatus status) {
        String message = switch (status) {
            case ExecutionStatus.Failed f -> f.message();
            case ExecutionStatus.Abandoned a -> a.message();
            default -> null;
        };
        String sql = """
            UPDATE job_executions SET status = ?, error_message = COALESCE(?, error_message),
              started_at = CASE WHEN ? = 'RUNNING' AND started_at IS NULL THEN now() ELSE started_at END,
              finished_at = CASE WHEN ? IN ('COMPLETED','FAILED','ABANDONED') THEN now() ELSE finished_at END
            WHERE id = ?
            """;
        String v = status.toDbValue();
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setString(1, v);
            ps.setString(2, message);
            ps.setString(3, v);
            ps.setString(4, v);
            ps.setObject(5, id);
            ps.executeUpdate();
        } catch (SQLException ex) {
            throw new RepositoryException("Failed to update job execution status " + id, ex);
        }
    }

    public List<JobExecution> findByJobName(String jobName, int limit) {
        String sql = "SELECT " + COLS + " FROM job_executions WHERE job_name = ? ORDER BY created_at DESC LIMIT ?";
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setString(1, jobName);
            ps.setInt(2, limit);
            try (ResultSet rs = ps.executeQuery()) {
                List<JobExecution> out = new ArrayList<>();
                while (rs.next()) out.add(map(rs));
                return out;
            }
        } catch (SQLException | JsonProcessingException ex) {
            throw new RepositoryException("Failed to list job executions for " + jobName, ex);
        }
    }

    private JobExecution map(ResultSet rs) throws SQLException, JsonProcessingException {
        String params = rs.getString(4);
        Map<String, Object> parameters = params == null ? Map.of()
            : MAPPER.readValue(params, new TypeReference<Map<String, Object>>() {});
        String msg = rs.getString(8);
        Timestamp s = rs.getTimestamp(5), f = rs.getTimestamp(6);
        return new JobExecution(rs.getObject(1, UUID.class), rs.getString(2),
            ExecutionStatus.fromDbValue(rs.getString(3), msg), parameters,
            s == null ? null : s.toInstant(), f == null ? null : f.toInstant(),
            ExecutionMetrics.empty(), rs.getInt(7), msg);
    }

    private static Timestamp ts(Instant i) {
        return i == null ? null : Timestamp.from(i);
    }
}
