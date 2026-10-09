package com.batchforge.engine.dag;

import com.batchforge.engine.core.ReaderConfig;
import com.batchforge.engine.core.WriterConfig;
import com.batchforge.readers.*;
import com.batchforge.writers.*;
import com.batchforge.spi.*;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import javax.sql.DataSource;

@ApplicationScoped
public class DefaultReaderWriterFactory implements ReaderWriterFactory {

    @Inject DataSource dataSource;

    @Override
    public ItemReader<?> createReader(ReaderConfig config) {
        return switch (config.type()) {
            case "jdbc" -> new JdbcReader(dataSource, config.properties().get("query"));
            case "csv" -> new CsvReader(config.properties().get("path"));
            case "jsonlines" -> new JsonLinesReader(config.properties().get("path"));
            default -> throw new IllegalArgumentException("Unknown reader type: " + config.type());
        };
    }

    @Override
    public ItemWriter<?> createWriter(WriterConfig config) {
        return switch (config.type()) {
            case "jdbc" -> new JdbcWriter(dataSource, config.properties().get("table"));
            case "csv" -> new CsvWriter(config.properties().get("path"));
            case "file" -> new com.batchforge.writers.FileWriter(config.properties().get("path"));
            default -> throw new IllegalArgumentException("Unknown writer type: " + config.type());
        };
    }

    @Override
    public ItemProcessor<?, ?> createProcessor(String className) {
        try {
            Class<?> clazz = Class.forName(className);
            return (ItemProcessor<?, ?>) clazz.getDeclaredConstructor().newInstance();
        } catch (Exception e) {
            throw new IllegalArgumentException("Cannot instantiate processor: " + className, e);
        }
    }
}
