package com.batchforge.engine.partition;

import com.batchforge.spi.*;
import jakarta.enterprise.context.ApplicationScoped;
import java.util.*;

@ApplicationScoped
public class PartitionManager {
    public List<PartitionRange> createPartitions(int partitionCount, ExecutionContext context) {
        if (partitionCount <= 1) {
            return List.of(new PartitionRange("main", Map.of()));
        }
        return new RangePartitioner().partition(context, partitionCount);
    }
}
