package com.batchforge.engine.repository;

import com.batchforge.engine.core.*;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;

import javax.sql.DataSource;
import java.sql.*;
import java.util.*;

@ApplicationScoped
public class JobDefinitionRepository {

    @Inject DataSource dataSource;

    @Transactional
    public void save(JobDefinition job, String dagJson, String yamlHash) {
        String sql = """
            INSERT INTO job_definitions (name, description, schedule, yaml_hash, dag_json)
            VALUES (?, ?, ?, ?, ?::jsonb)
            ON CONFLICT (name) DO UPDATE SET
              description = EXCLUDED.description, schedule = EXCLUDED.schedule,
              yaml_hash = EXCLUDED.yaml_hash, dag_json = EXCLUDED.dag_json,
              updated_at = now()
            """;
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setString(1, job.name());
            ps.setString(2, job.description());
            ps.setString(3, job.schedule());
            ps.setString(4, yamlHash);
            ps.setString(5, dagJson);
            ps.executeUpdate();
        } catch (SQLException e) {
            throw new RepositoryException("Failed to save job definition " + job.name(), e);
        }
    }

    public Optional<JobDefinition> findByName(String name) {
        String sql = "SELECT name, description, schedule FROM job_definitions WHERE name = ?";
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql)) {
            ps.setString(1, name);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() ? Optional.of(map(rs)) : Optional.empty();
            }
        } catch (SQLException e) {
            throw new RepositoryException("Failed to find job definition " + name, e);
        }
    }

    public List<JobDefinition> findAll() {
        String sql = "SELECT name, description, schedule FROM job_definitions ORDER BY name";
        try (Connection c = dataSource.getConnection(); PreparedStatement ps = c.prepareStatement(sql);
             ResultSet rs = ps.executeQuery()) {
            List<JobDefinition> out = new ArrayList<>();
            while (rs.next()) out.add(map(rs));
            return out;
        } catch (SQLException e) {
            throw new RepositoryException("Failed to list job definitions", e);
        }
    }

    @Transactional
    public void delete(String name) {
        try (Connection c = dataSource.getConnection();
             PreparedStatement ps = c.prepareStatement("DELETE FROM job_definitions WHERE name = ?")) {
            ps.setString(1, name);
            ps.executeUpdate();
        } catch (SQLException e) {
            throw new RepositoryException("Failed to delete job definition " + name, e);
        }
    }

    /**
     * Full step definitions are resolved from YAML; the DB only keeps the DAG JSON.
     * A placeholder step is supplied because JobDefinition requires at least one step.
     */
    private JobDefinition map(ResultSet rs) throws SQLException {
        var step = new StepDefinition("unresolved",
            new ReaderConfig("unresolved", Map.of()), "unresolved",
            new WriterConfig("unresolved", Map.of()), 1, 1, List.of(), RetryConfig.none(), false);
        return new JobDefinition(rs.getString(1), rs.getString(2), rs.getString(3),
            Map.of(), Map.of(step.name(), step));
    }
}
