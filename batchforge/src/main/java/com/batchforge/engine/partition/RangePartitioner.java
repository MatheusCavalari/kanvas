package com.batchforge.engine.partition;

import com.batchforge.spi.*;
import java.util.*;

public class RangePartitioner implements Partitioner {
    @Override
    public List<PartitionRange> partition(ExecutionContext context, int partitionCount) {
        long minId = Long.parseLong(context.stepProperties().get("minId").toString());
        long maxId = Long.parseLong(context.stepProperties().get("maxId").toString());
        long range = maxId - minId;
        long partSize = range / partitionCount;

        List<PartitionRange> partitions = new ArrayList<>();
        for (int i = 0; i < partitionCount; i++) {
            long start = minId + (i * partSize);
            long end = (i == partitionCount - 1) ? maxId : start + partSize;
            partitions.add(new PartitionRange(
                "partition-" + i,
                Map.of("rangeStart", start, "rangeEnd", end)
            ));
        }
        return partitions;
    }
}
