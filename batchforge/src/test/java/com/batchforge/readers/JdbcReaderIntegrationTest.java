package com.batchforge.readers;

import com.batchforge.spi.ExecutionContext;
import org.junit.jupiter.api.Test;
import org.postgresql.ds.PGSimpleDataSource;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.util.*;

import static org.assertj.core.api.Assertions.*;

@Testcontainers
class JdbcReaderIntegrationTest {

    @Container
    static PostgreSQLContainer<?> pg = new PostgreSQLContainer<>("postgres:16-alpine");

    @Test
    void readsRowsAsMaps() throws Exception {
        var ds = new PGSimpleDataSource();
        ds.setUrl(pg.getJdbcUrl());
        ds.setUser(pg.getUsername());
        ds.setPassword(pg.getPassword());
        try (var c = ds.getConnection(); var st = c.createStatement()) {
            st.execute("CREATE TABLE people (id INT, name TEXT)");
            st.execute("INSERT INTO people VALUES (1,'Alice'),(2,'Bob'),(3,'Carol')");
        }

        var reader = new JdbcReader(ds, "SELECT id, name FROM people ORDER BY id");
        reader.open(new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of()));
        var chunk = reader.read(2);
        assertThat(chunk).hasSize(2);
        assertThat(chunk.getFirst()).containsEntry("id", 1).containsEntry("name", "Alice");
        assertThat(reader.read(2)).hasSize(1);
        assertThat(reader.hasMore()).isFalse();
        reader.close();
    }
}
