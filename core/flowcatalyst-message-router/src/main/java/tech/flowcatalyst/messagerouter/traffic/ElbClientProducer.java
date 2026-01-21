package tech.flowcatalyst.messagerouter.traffic;

import io.quarkus.arc.DefaultBean;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Produces;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import software.amazon.awssdk.auth.credentials.DefaultCredentialsProvider;
import software.amazon.awssdk.regions.Region;
import software.amazon.awssdk.services.elasticloadbalancingv2.ElasticLoadBalancingV2Client;

import java.net.URI;
import java.util.Optional;
import java.util.logging.Logger;

/**
 * CDI producer for AWS ELB v2 client.
 *
 * Creates an ElasticLoadBalancingV2Client configured with the same
 * region and credentials as the SQS client (via DefaultCredentialsProvider).
 *
 * The client is only created when traffic-management.strategy=aws-alb.
 */
@ApplicationScoped
public class ElbClientProducer {

    private static final Logger LOG = Logger.getLogger(ElbClientProducer.class.getName());

    @Inject
    TrafficManagementConfig trafficConfig;

    @ConfigProperty(name = "quarkus.sqs.aws.region", defaultValue = "eu-west-1")
    String awsRegion;

    @ConfigProperty(name = "quarkus.sqs.endpoint-override")
    Optional<String> endpointOverride;

    @Produces
    @ApplicationScoped
    @DefaultBean
    public ElasticLoadBalancingV2Client createElbClient() {
        LOG.info("Creating ELB v2 client for region: " + awsRegion);

        var builder = ElasticLoadBalancingV2Client.builder()
            .region(Region.of(awsRegion))
            .credentialsProvider(DefaultCredentialsProvider.create());

        // Use endpoint override if configured (for LocalStack testing)
        endpointOverride.ifPresent(endpoint -> {
            LOG.info("Using ELB endpoint override: " + endpoint);
            builder.endpointOverride(URI.create(endpoint));
        });

        return builder.build();
    }
}
