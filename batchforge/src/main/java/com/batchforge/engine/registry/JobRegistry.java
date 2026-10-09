package com.batchforge.engine.registry;

import com.batchforge.engine.core.JobDefinition;
import com.batchforge.engine.dag.DagValidator;
import com.batchforge.engine.repository.JobDefinitionRepository;
import com.batchforge.yaml.JobDefinitionParser;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;

import java.io.IOException;
import java.nio.file.*;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;

@ApplicationScoped
public class JobRegistry {

    private static final Logger LOG = Logger.getLogger(JobRegistry.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private final Map<String, JobDefinition> jobs = new ConcurrentHashMap<>();
    private final JobDefinitionParser parser;
    private final DagValidator validator;
    private final JobDefinitionRepository repository;

    @Inject
    public JobRegistry(JobDefinitionParser parser, DagValidator validator, JobDefinitionRepository repository) {
        this.parser = parser;
        this.validator = validator;
        this.repository = repository;
    }

    public void register(JobDefinition job) {
        var errors = validator.validate(job);
        if (!errors.isEmpty()) {
            LOG.warnf("Job '%s' has validation errors: %s", job.name(), errors);
            return;
        }
        jobs.put(job.name(), job);
        try {
            String dagJson = MAPPER.writeValueAsString(validator.topologicalSort(job));
            repository.save(job, dagJson, Integer.toHexString(job.hashCode()));
        } catch (Exception e) {
            LOG.warnf("Failed to persist job '%s': %s", job.name(), e.getMessage());
        }
        LOG.infof("Registered job '%s' with %d steps", job.name(), job.steps().size());
    }

    public Optional<JobDefinition> getJob(String name) {
        return Optional.ofNullable(jobs.get(name));
    }

    public List<JobDefinition> listJobs() {
        return List.copyOf(jobs.values());
    }

    public void loadFromDirectory(Path dir) {
        try (var stream = Files.list(dir)) {
            stream.filter(JobRegistry::isYaml).forEach(file -> {
                try {
                    register(parser.parseFile(file));
                } catch (Exception e) {
                    LOG.warnf("Failed to load job from '%s': %s", file, e.getMessage());
                }
            });
        } catch (IOException e) {
            LOG.errorf("Failed to scan directory '%s': %s", dir, e.getMessage());
        }
    }

    public void startWatching(Path dir) {
        Thread.ofVirtual().name("job-watcher").start(() -> {
            try (WatchService watcher = FileSystems.getDefault().newWatchService()) {
                dir.register(watcher, StandardWatchEventKinds.ENTRY_CREATE, StandardWatchEventKinds.ENTRY_MODIFY);
                while (!Thread.currentThread().isInterrupted()) {
                    WatchKey key = watcher.take();
                    for (WatchEvent<?> event : key.pollEvents()) {
                        if (!(event.context() instanceof Path rel)) continue;
                        Path file = dir.resolve(rel);
                        if (isYaml(file)) {
                            LOG.infof("Detected change in '%s', reloading", file);
                            try {
                                register(parser.parseFile(file));
                            } catch (Exception e) {
                                LOG.warnf("Failed to reload '%s': %s", file, e.getMessage());
                            }
                        }
                    }
                    key.reset();
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            } catch (IOException e) {
                LOG.errorf("Watcher failed: %s", e.getMessage());
            }
        });
    }

    private static boolean isYaml(Path p) {
        String s = p.toString();
        return s.endsWith(".yml") || s.endsWith(".yaml");
    }
}
