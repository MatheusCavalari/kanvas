package com.batchforge.readers;

import com.batchforge.spi.*;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.*;
import java.nio.file.*;
import java.util.*;

public class JsonLinesReader implements ItemReader<Map<String, Object>> {
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final TypeReference<LinkedHashMap<String, Object>> TYPE = new TypeReference<>() {};

    private final String path;
    private String resolvedPath;
    private BufferedReader reader;
    private boolean done;

    public JsonLinesReader(String path) { this.path = path; }

    @Override
    public void open(ExecutionContext context) {
        try {
            resolvedPath = context.resolveTemplate(path);
            reader = Files.newBufferedReader(Path.of(resolvedPath));
            done = false;
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public List<Map<String, Object>> read(int chunkSize) {
        List<Map<String, Object>> rows = new ArrayList<>();
        try {
            while (rows.size() < chunkSize) {
                String line = reader.readLine();
                if (line == null) { done = true; break; }
                if (line.isBlank()) continue;
                rows.add(MAPPER.readValue(line, TYPE));
            }
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
        return rows;
    }

    @Override
    public boolean hasMore() { return !done; }

    @Override
    public void seekTo(long offset) {
        try {
            if (reader != null) reader.close();
            reader = Files.newBufferedReader(Path.of(resolvedPath));
            done = false;
            long skipped = 0;
            while (skipped < offset) {
                String line = reader.readLine();
                if (line == null) { done = true; break; }
                if (!line.isBlank()) skipped++;
            }
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public void close() {
        try { if (reader != null) reader.close(); }
        catch (IOException e) { throw new UncheckedIOException(e); }
    }
}
