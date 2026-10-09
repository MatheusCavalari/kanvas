package com.batchforge.readers;

import com.batchforge.spi.*;
import javax.sql.DataSource;
import java.sql.*;
import java.util.*;

public class JdbcReader implements ItemReader<Map<String, Object>> {
    private final DataSource dataSource;
    private final String query;
    private Connection connection;
    private PreparedStatement statement;
    private ResultSet resultSet;
    private boolean done;

    public JdbcReader(DataSource dataSource, String query) {
        this.dataSource = dataSource;
        this.query = query;
    }

    @Override
    public void open(ExecutionContext context) {
        try {
            closeQuietly();
            connection = dataSource.getConnection();
            statement = connection.prepareStatement(context.resolveTemplate(query));
            resultSet = statement.executeQuery();
            done = false;
        } catch (SQLException e) {
            throw new IllegalStateException("Failed to execute query", e);
        }
    }

    @Override
    public List<Map<String, Object>> read(int chunkSize) {
        List<Map<String, Object>> rows = new ArrayList<>();
        try {
            ResultSetMetaData md = resultSet.getMetaData();
            int cols = md.getColumnCount();
            while (rows.size() < chunkSize) {
                if (!resultSet.next()) { done = true; break; }
                Map<String, Object> row = new LinkedHashMap<>();
                for (int i = 1; i <= cols; i++) row.put(md.getColumnLabel(i), resultSet.getObject(i));
                rows.add(row);
            }
        } catch (SQLException e) {
            throw new IllegalStateException("Failed to read rows", e);
        }
        return rows;
    }

    @Override
    public boolean hasMore() { return !done; }

    @Override
    public void seekTo(long offset) {
        try {
            for (long i = 0; i < offset; i++) {
                if (!resultSet.next()) { done = true; break; }
            }
        } catch (SQLException e) {
            throw new IllegalStateException("Failed to seek", e);
        }
    }

    @Override
    public void close() { closeQuietly(); }

    private void closeQuietly() {
        try { if (resultSet != null) resultSet.close(); } catch (SQLException ignored) {}
        try { if (statement != null) statement.close(); } catch (SQLException ignored) {}
        try { if (connection != null) connection.close(); } catch (SQLException ignored) {}
        resultSet = null; statement = null; connection = null;
    }
}
