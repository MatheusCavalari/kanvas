package com.batchforge.engine;

import com.batchforge.spi.ItemProcessor;
import java.util.Map;

public class TestPassthroughProcessor implements ItemProcessor<Map<String, Object>, Map<String, Object>> {
    @Override
    public Map<String, Object> process(Map<String, Object> item) {
        return item;
    }
}
