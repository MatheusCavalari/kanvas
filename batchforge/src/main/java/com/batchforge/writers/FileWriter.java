package com.batchforge.writers;

import com.batchforge.spi.*;
import java.io.*;
import java.nio.file.*;
import java.util.*;

public class FileWriter implements ItemWriter<String> {
    private final String path;
    private BufferedWriter writer;

    public FileWriter(String path) { this.path = path; }

    @Override
    public void open(ExecutionContext context) {
        try {
            writer = Files.newBufferedWriter(Path.of(context.resolveTemplate(path)),
                StandardOpenOption.CREATE, StandardOpenOption.APPEND);
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public void write(List<String> items) {
        try {
            for (String item : items) {
                writer.write(item);
                writer.newLine();
            }
            writer.flush();
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    @Override
    public void close() {
        try { if (writer != null) writer.close(); }
        catch (IOException e) { throw new UncheckedIOException(e); }
    }
}
