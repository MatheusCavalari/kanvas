package com.batchforge.engine;

import com.batchforge.engine.core.JobExecution;
import com.batchforge.engine.dag.DagScheduler;
import com.batchforge.engine.registry.JobRegistry;
import com.batchforge.engine.repository.JobExecutionRepository;
import com.batchforge.yaml.JobDefinitionParser;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;

import java.nio.file.Path;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

@ApplicationScoped
public class BatchForgeEngine {

    private static final Logger LOG = Logger.getLogger(BatchForgeEngine.class);

    @Inject JobRegistry registry;
    @Inject DagScheduler scheduler;
    @Inject JobExecutionRepository jobExecRepo;
    @Inject JobDefinitionParser parser;

    @ConfigProperty(name = "batchforge.jobs-dir", defaultValue = "jobs")
    String jobsDir;

    public void start() {
        Path dir = Path.of(jobsDir);
        if (dir.toFile().exists()) {
            registry.loadFromDirectory(dir);
            registry.startWatching(dir);
            LOG.infof("BatchForge started, watching '%s'", dir);
        }
    }

    public void loadJobFile(Path file) throws Exception {
        registry.register(parser.parseFile(file));
    }

    public UUID executeJob(String jobName, Map<String, Object> parameters) {
        var jobDef = registry.getJob(jobName)
            .orElseThrow(() -> new IllegalArgumentException("Job not found: " + jobName));

        var execution = jobExecRepo.save(JobExecution.create(jobName, parameters));

        Thread.ofVirtual().name("job-" + execution.id()).start(() -> {
            try {
                scheduler.execute(jobDef, execution);
            } catch (Exception e) {
                LOG.errorf("Job '%s' execution %s failed: %s", jobName, execution.id(), e.getMessage());
            }
        });

        return execution.id();
    }

    public Optional<JobExecution> getExecution(UUID id) {
        return jobExecRepo.findById(id);
    }
}
