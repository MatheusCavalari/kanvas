package com.batchforge.writers;

import com.batchforge.spi.ExecutionContext;
import org.junit.jupiter.api.Test;
import org.postgresql.ds.PGSimpleDataSource;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

import java.util.*;

import static org.assertj.core.api.Assertions.*;

@Testcontainers
class JdbcWriterIntegrationTest {

    @Container
    static PostgreSQLContainer<?> pg = new PostgreSQLContainer<>("postgres:16-alpine");

    @Test
    void insertsRows() throws Exception {
        var ds = new PGSimpleDataSource();
        ds.setUrl(pg.getJdbcUrl());
        ds.setUser(pg.getUsername());
        ds.setPassword(pg.getPassword());
        try (var c = ds.getConnection(); var st = c.createStatement()) {
            st.execute("CREATE TABLE out_people (id INT, name TEXT)");
        }

        var writer = new JdbcWriter(ds, "out_people");
        writer.open(new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of()));
        writer.write(List.of(Map.of("id", 1, "name", "A"), Map.of("id", 2, "name", "B")));
        writer.close();

        try (var c = ds.getConnection(); var st = c.createStatement();
             var rs = st.executeQuery("SELECT count(*) FROM out_people")) {
            rs.next();
            assertThat(rs.getInt(1)).isEqualTo(2);
        }
    }
}
