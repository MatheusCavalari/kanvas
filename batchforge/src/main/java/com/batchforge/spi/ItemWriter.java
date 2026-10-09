package com.batchforge.spi;

import java.util.List;

public interface ItemWriter<T> {
    void open(ExecutionContext context);
    void write(List<T> items);
    void close();
}
