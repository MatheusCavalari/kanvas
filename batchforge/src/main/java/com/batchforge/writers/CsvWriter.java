package com.batchforge.writers;

import com.batchforge.spi.*;
import java.io.*;
import java.nio.file.*;
import java.util.*;
import java.util.stream.Collectors;

public class CsvWriter implements ItemWriter<Map<String, Object>> {
    private final String path;
    private BufferedWriter writer;
    private List<String> headers;

    public CsvWriter(String path) { this.path = path; }

    @Override
    public void open(ExecutionContext context) {
        try {
            writer = Files.newBufferedWriter(Path.of(context.resolveTemplate(path)));
            headers = null;
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public void write(List<Map<String, Object>> items) {
        if (items.isEmpty()) return;
        try {
            if (headers == null) {
                headers = new ArrayList<>(items.getFirst().keySet());
                writer.write(String.join(",", headers));
                writer.newLine();
            }
            for (Map<String, Object> item : items) {
                writer.write(headers.stream().map(h -> escape(item.get(h))).collect(Collectors.joining(",")));
                writer.newLine();
            }
            writer.flush();
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    private static String escape(Object v) {
        if (v == null) return "";
        String s = v.toString();
        if (s.contains(",") || s.contains("\"") || s.contains("\n")) {
            return "\"" + s.replace("\"", "\"\"") + "\"";
        }
        return s;
    }

    @Override
    public void close() {
        try { if (writer != null) writer.close(); }
        catch (IOException e) { throw new UncheckedIOException(e); }
    }
}
