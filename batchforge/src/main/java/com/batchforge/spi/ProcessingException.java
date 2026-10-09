package com.batchforge.spi;

public class ProcessingException extends Exception {
    public ProcessingException(String message) { super(message); }
    public ProcessingException(String message, Throwable cause) { super(message, cause); }
}
