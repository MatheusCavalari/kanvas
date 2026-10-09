package com.batchforge.spi;

import java.util.List;

public interface ItemReader<T> {
    void open(ExecutionContext context);
    List<T> read(int chunkSize);
    boolean hasMore();
    void seekTo(long offset);
    void close();
}
