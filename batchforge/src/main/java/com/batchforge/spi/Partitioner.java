package com.batchforge.spi;

import java.util.List;

public interface Partitioner {
    List<PartitionRange> partition(ExecutionContext context, int partitionCount);
}
