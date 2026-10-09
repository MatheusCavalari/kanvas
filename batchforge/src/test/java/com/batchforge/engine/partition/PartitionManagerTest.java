package com.batchforge.engine.partition;

import com.batchforge.spi.*;
import org.junit.jupiter.api.Test;
import java.util.*;

import static org.assertj.core.api.Assertions.*;

class PartitionManagerTest {

    @Test
    void rangePartitionerSplitsEvenly() {
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of(), Map.of("minId", "0", "maxId", "1000"));
        var partitioner = new RangePartitioner();
        var partitions = partitioner.partition(ctx, 4);

        assertThat(partitions).hasSize(4);
        assertThat(partitions.get(0).parameters()).containsEntry("rangeStart", 0L);
        assertThat(partitions.get(0).parameters()).containsEntry("rangeEnd", 250L);
        assertThat(partitions.get(3).parameters()).containsEntry("rangeStart", 750L);
        assertThat(partitions.get(3).parameters()).containsEntry("rangeEnd", 1000L);
    }

    @Test
    void singlePartitionReturnsMainOnly() {
        var manager = new PartitionManager();
        var ctx = new ExecutionContext(UUID.randomUUID(), UUID.randomUUID(), "s1", "main",
            Map.of(), Map.of());
        var partitions = manager.createPartitions(1, ctx);

        assertThat(partitions).hasSize(1);
        assertThat(partitions.getFirst().partitionId()).isEqualTo("main");
    }
}
