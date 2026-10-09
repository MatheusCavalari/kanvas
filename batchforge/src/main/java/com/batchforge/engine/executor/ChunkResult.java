package com.batchforge.engine.executor;

import java.time.Duration;

public record ChunkResult(int itemsRead, int itemsWritten, int itemsFailed, Duration duration) {}
