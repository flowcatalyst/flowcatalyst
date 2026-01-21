package tech.flowcatalyst.outbox.repository;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Instance;
import jakarta.enterprise.inject.Produces;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.config.DatabaseType;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.repository.mongo.MongoOutboxRepository;
import tech.flowcatalyst.outbox.repository.mysql.MysqlOutboxRepository;
import tech.flowcatalyst.outbox.repository.postgres.PostgresOutboxRepository;

/**
 * CDI producer that selects the appropriate OutboxRepository implementation
 * based on the configured database type.
 */
@ApplicationScoped
public class OutboxRepositoryProducer {

    private static final Logger LOG = Logger.getLogger(OutboxRepositoryProducer.class);

    @Inject
    OutboxProcessorConfig config;

    @Inject
    Instance<PostgresOutboxRepository> postgresRepo;

    @Inject
    Instance<MysqlOutboxRepository> mysqlRepo;

    @Inject
    Instance<MongoOutboxRepository> mongoRepo;

    @Produces
    @ApplicationScoped
    public OutboxRepository produceRepository() {
        DatabaseType dbType = config.databaseType();
        LOG.infof("Configuring OutboxRepository for database type: %s", dbType);

        return switch (dbType) {
            case POSTGRESQL -> {
                if (!postgresRepo.isResolvable()) {
                    throw new IllegalStateException(
                        "PostgreSQL repository not available. Ensure quarkus-jdbc-postgresql is on classpath.");
                }
                yield postgresRepo.get();
            }
            case MYSQL -> {
                if (!mysqlRepo.isResolvable()) {
                    throw new IllegalStateException(
                        "MySQL repository not available. Ensure quarkus-jdbc-mysql is on classpath.");
                }
                yield mysqlRepo.get();
            }
            case MONGODB -> {
                if (!mongoRepo.isResolvable()) {
                    throw new IllegalStateException(
                        "MongoDB repository not available. Ensure quarkus-mongodb-client is on classpath.");
                }
                yield mongoRepo.get();
            }
        };
    }
}
