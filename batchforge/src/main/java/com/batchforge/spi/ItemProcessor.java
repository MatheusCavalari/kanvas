package com.batchforge.spi;

public interface ItemProcessor<I, O> {
    O process(I item) throws ProcessingException;
}
