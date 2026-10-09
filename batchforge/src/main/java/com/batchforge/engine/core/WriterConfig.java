package com.batchforge.engine.core;

import java.util.Map;

public record WriterConfig(String type, Map<String, String> properties) {}
