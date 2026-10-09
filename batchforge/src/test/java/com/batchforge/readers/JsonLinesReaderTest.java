package com.batchforge.readers;

import com.batchforge.spi.ExecutionContext;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.*;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class JsonLinesReaderTest {

    @TempDir Path tempDir;

    @Test
    void readsAndSeeks() throws Exception {
        Path f = tempDir.resolve("data.jsonl");
        Files.writeString(f, "{\"id\":1,\"name\":\"A\"}\n{\"id\":2,\"name\":\"B\"}\n{\"id\":3,\"name\":\"C\"}\n");
        var reader = new JsonLinesReader(f.toString());
        reader.open(new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main", Map.of(), Map.of()));

        var chunk = reader.read(2);
        assertThat(chunk).hasSize(2);
        assertThat(chunk.getFirst()).containsEntry("id", 1).containsEntry("name", "A");

        reader.seekTo(2);
        var rest = reader.read(10);
        assertThat(rest).hasSize(1);
        assertThat(rest.getFirst()).containsEntry("name", "C");
        assertThat(reader.hasMore()).isFalse();
        reader.close();
    }
}
