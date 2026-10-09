package com.batchforge.writers;

import com.batchforge.spi.*;
import javax.sql.DataSource;
import java.sql.*;
import java.util.*;
import java.util.stream.Collectors;

public class JdbcWriter implements ItemWriter<Map<String, Object>> {
    private final DataSource dataSource;
    private final String table;
    private Connection connection;

    public JdbcWriter(DataSource dataSource, String table) {
        this.dataSource = dataSource;
        this.table = quote(table);
    }

    private static final java.util.regex.Pattern IDENTIFIER = java.util.regex.Pattern.compile("[a-zA-Z0-9_]+");

    static String quote(String identifier) {
        if (identifier == null || !IDENTIFIER.matcher(identifier).matches()) {
            throw new IllegalArgumentException("Invalid SQL identifier: " + identifier);
        }
        return "\"" + identifier + "\"";
    }

    @Override
    public void open(ExecutionContext context) {
        try {
            connection = dataSource.getConnection();
            connection.setAutoCommit(false);
        } catch (SQLException e) {
            throw new IllegalStateException("Failed to open connection", e);
        }
    }

    @Override
    public void write(List<Map<String, Object>> items) {
        if (items.isEmpty()) return;
        List<String> columns = new ArrayList<>(items.getFirst().keySet());
        String sql = "INSERT INTO " + table + " ("
            + columns.stream().map(JdbcWriter::quote).collect(Collectors.joining(", ")) + ") VALUES ("
            + columns.stream().map(c -> "?").collect(Collectors.joining(", ")) + ")";
        try (PreparedStatement ps = connection.prepareStatement(sql)) {
            for (Map<String, Object> item : items) {
                for (int i = 0; i < columns.size(); i++) ps.setObject(i + 1, item.get(columns.get(i)));
                ps.addBatch();
            }
            ps.executeBatch();
            connection.commit();
        } catch (SQLException e) {
            try { connection.rollback(); } catch (SQLException ignored) {}
            throw new IllegalStateException("Batch insert failed", e);
        }
    }

    @Override
    public void close() {
        try { if (connection != null) connection.close(); }
        catch (SQLException e) { throw new IllegalStateException(e); }
    }
}
