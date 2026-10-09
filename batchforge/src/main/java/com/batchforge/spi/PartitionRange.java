package com.batchforge.spi;

import java.util.Map;

public record PartitionRange(String partitionId, Map<String, Object> parameters) {}
