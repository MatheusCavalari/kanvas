package com.batchforge.readers;

import com.batchforge.spi.*;
import java.io.*;
import java.nio.file.*;
import java.util.*;

public class CsvReader implements ItemReader<Map<String, Object>> {
    private final String path;
    private String resolvedPath;
    private BufferedReader bufferedReader;
    private String[] headers;
    private long currentOffset;
    private boolean done;

    public CsvReader(String path) { this.path = path; }

    @Override
    public void open(ExecutionContext context) {
        try {
            resolvedPath = context.resolveTemplate(path);
            bufferedReader = Files.newBufferedReader(Path.of(resolvedPath));
            String headerLine = bufferedReader.readLine();
            if (headerLine == null) throw new IllegalStateException("CSV is empty");
            headers = headerLine.split(",");
            currentOffset = 0;
            done = false;
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public List<Map<String, Object>> read(int chunkSize) {
        List<Map<String, Object>> rows = new ArrayList<>();
        try {
            for (int i = 0; i < chunkSize; i++) {
                String line = bufferedReader.readLine();
                if (line == null) { done = true; break; }
                String[] values = line.split(",", -1);
                Map<String, Object> row = new LinkedHashMap<>();
                for (int j = 0; j < headers.length && j < values.length; j++) {
                    row.put(headers[j].trim(), values[j].trim());
                }
                rows.add(row);
                currentOffset++;
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
            if (bufferedReader != null) bufferedReader.close();
            bufferedReader = Files.newBufferedReader(Path.of(resolvedPath));
            bufferedReader.readLine();
            done = false;
            for (long i = 0; i < offset; i++) {
                if (bufferedReader.readLine() == null) { done = true; break; }
            }
            currentOffset = offset;
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public void close() {
        try { if (bufferedReader != null) bufferedReader.close(); }
        catch (IOException e) { throw new UncheckedIOException(e); }
    }
}
