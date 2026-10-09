package com.batchforge.readers;

import com.batchforge.spi.ExecutionContext;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.*;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class CsvReaderTest {

    @TempDir Path tempDir;

    @Test
    void readsAllRowsWithHeaders() throws Exception {
        Path csv = tempDir.resolve("data.csv");
        Files.writeString(csv, "id,name,amount\n1,Alice,100.50\n2,Bob,200.75\n3,Carol,50.00\n");

        var reader = new CsvReader(csv.toString());
        reader.open(new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of()));

        var chunk = reader.read(2);
        assertThat(chunk).hasSize(2);
        assertThat(chunk.getFirst()).containsEntry("id", "1").containsEntry("name", "Alice");

        var chunk2 = reader.read(2);
        assertThat(chunk2).hasSize(1);
        assertThat(chunk2.getFirst()).containsEntry("id", "3");

        assertThat(reader.hasMore()).isFalse();
        reader.close();
    }

    @Test
    void seekToSkipsRows() throws Exception {
        Path csv = tempDir.resolve("data.csv");
        Files.writeString(csv, "id,name\n1,A\n2,B\n3,C\n4,D\n");

        var reader = new CsvReader(csv.toString());
        reader.open(new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of()));
        reader.seekTo(2);

        var chunk = reader.read(10);
        assertThat(chunk).hasSize(2);
        assertThat(chunk.getFirst()).containsEntry("id", "3");
        reader.close();
    }
}
