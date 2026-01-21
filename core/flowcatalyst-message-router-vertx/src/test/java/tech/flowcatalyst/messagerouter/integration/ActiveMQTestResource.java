package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.common.QuarkusTestResourceLifecycleManager;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.wait.strategy.Wait;
import org.testcontainers.utility.DockerImageName;

import java.time.Duration;
import java.util.Map;

/**
 * Quarkus test resource that starts ActiveMQ Classic container for integration tests.
 */
public class ActiveMQTestResource implements QuarkusTestResourceLifecycleManager {

    private static final DockerImageName ACTIVEMQ_IMAGE =
        DockerImageName.parse("apache/activemq-classic:latest");

    private static final String DEFAULT_USERNAME = "admin";
    private static final String DEFAULT_PASSWORD = "admin";

    private GenericContainer<?> activemqContainer;

    @Override
    public Map<String, String> start() {
        activemqContainer = new GenericContainer<>(ACTIVEMQ_IMAGE)
            .withExposedPorts(61616)
            .waitingFor(Wait.forListeningPort().withStartupTimeout(Duration.ofMinutes(2)))
            .withReuse(true);

        activemqContainer.start();

        return Map.of(
            "activemq.broker.url",
                String.format("tcp://%s:%d",
                    activemqContainer.getHost(),
                    activemqContainer.getMappedPort(61616)),
            "activemq.username", DEFAULT_USERNAME,
            "activemq.password", DEFAULT_PASSWORD
        );
    }

    @Override
    public void stop() {
        if (activemqContainer != null) {
            activemqContainer.stop();
        }
    }
}
