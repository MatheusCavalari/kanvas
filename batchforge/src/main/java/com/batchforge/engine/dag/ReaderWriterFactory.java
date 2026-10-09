package com.batchforge.engine.dag;

import com.batchforge.engine.core.ReaderConfig;
import com.batchforge.engine.core.WriterConfig;
import com.batchforge.spi.*;

public interface ReaderWriterFactory {
    ItemReader<?> createReader(ReaderConfig config);
    ItemWriter<?> createWriter(WriterConfig config);
    ItemProcessor<?, ?> createProcessor(String className);
}
