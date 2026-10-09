package com.batchforge.engine.dag;

import com.batchforge.engine.core.JobDefinition;

import java.util.*;

@jakarta.enterprise.context.ApplicationScoped
public class DagValidator {

    public List<String> validate(JobDefinition job) {
        List<String> errors = new ArrayList<>();
        Set<String> stepNames = job.steps().keySet();

        for (var entry : job.steps().entrySet()) {
            for (String dep : entry.getValue().dependsOn()) {
                if (!stepNames.contains(dep)) {
                    errors.add("Step '" + entry.getKey() + "' depends on non-existent step '" + dep + "'");
                }
            }
        }

        if (!errors.isEmpty()) return errors;

        Map<String, Set<String>> inEdges = new LinkedHashMap<>();
        Map<String, Set<String>> outEdges = new LinkedHashMap<>();
        buildEdges(job, inEdges, outEdges);

        Queue<String> queue = new ArrayDeque<>();
        for (var entry : inEdges.entrySet()) {
            if (entry.getValue().isEmpty()) queue.add(entry.getKey());
        }

        int visited = 0;
        while (!queue.isEmpty()) {
            String node = queue.poll();
            visited++;
            for (String next : outEdges.get(node)) {
                inEdges.get(next).remove(node);
                if (inEdges.get(next).isEmpty()) queue.add(next);
            }
        }

        if (visited != stepNames.size()) {
            errors.add("Job '" + job.name() + "' has a dependency cycle");
        }

        return errors;
    }

    public List<List<String>> topologicalSort(JobDefinition job) {
        List<String> errors = validate(job);
        if (!errors.isEmpty()) throw new IllegalArgumentException("Invalid DAG: " + errors);

        Map<String, Set<String>> inEdges = new LinkedHashMap<>();
        Map<String, Set<String>> outEdges = new LinkedHashMap<>();
        buildEdges(job, inEdges, outEdges);

        List<List<String>> levels = new ArrayList<>();
        Set<String> remaining = new LinkedHashSet<>(job.steps().keySet());

        while (!remaining.isEmpty()) {
            List<String> level = new ArrayList<>();
            for (String name : remaining) {
                if (inEdges.get(name).isEmpty()) level.add(name);
            }
            if (level.isEmpty()) throw new IllegalStateException("Unexpected cycle");
            levels.add(level);
            remaining.removeAll(level);
            for (String done : level) {
                for (String next : outEdges.get(done)) {
                    inEdges.get(next).remove(done);
                }
            }
        }

        return levels;
    }

    private void buildEdges(JobDefinition job, Map<String, Set<String>> inEdges, Map<String, Set<String>> outEdges) {
        for (String name : job.steps().keySet()) {
            inEdges.put(name, new LinkedHashSet<>());
            outEdges.put(name, new LinkedHashSet<>());
        }
        for (var entry : job.steps().entrySet()) {
            for (String dep : entry.getValue().dependsOn()) {
                inEdges.get(entry.getKey()).add(dep);
                outEdges.get(dep).add(entry.getKey());
            }
        }
    }
}
