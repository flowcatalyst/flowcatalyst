package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.common.QuarkusTestResourceLifecycleManager;
import org.testcontainers.containers.localstack.LocalStackContainer;
import org.testcontainers.containers.wait.strategy.Wait;
import org.testcontainers.utility.DockerImageName;

import java.time.Duration;
import java.util.Map;

import static org.testcontainers.containers.localstack.LocalStackContainer.Service.SQS;

/**
 * Quarkus test resource that starts LocalStack container for SQS integration tests.
 */
public class LocalStackTestResource implements QuarkusTestResourceLifecycleManager {

    private static final DockerImageName LOCALSTACK_IMAGE =
        DockerImageName.parse("localstack/localstack:3.0");

    private LocalStackContainer localStackContainer;

    @Override
    public Map<String, String> start() {
        localStackContainer = new LocalStackContainer(LOCALSTACK_IMAGE)
            .withServices(SQS)
            .withReuse(true)
            .waitingFor(Wait.forHealthcheck().withStartupTimeout(Duration.ofMinutes(2)));

        localStackContainer.start();

        return Map.of(
            "quarkus.sqs.endpoint-override", localStackContainer.getEndpointOverride(SQS).toString(),
            "quarkus.sqs.aws.region", localStackContainer.getRegion(),
            "quarkus.sqs.aws.credentials.type", "static",
            "quarkus.sqs.aws.credentials.static-provider.access-key-id", localStackContainer.getAccessKey(),
            "quarkus.sqs.aws.credentials.static-provider.secret-access-key", localStackContainer.getSecretKey()
        );
    }

    @Override
    public void stop() {
        if (localStackContainer != null) {
            localStackContainer.stop();
        }
    }
}
