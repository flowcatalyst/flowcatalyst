package tech.flowcatalyst.messagerouter.config;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Produces;
import jakarta.jms.ConnectionFactory;
import org.apache.activemq.ActiveMQConnectionFactory;
import org.eclipse.microprofile.config.inject.ConfigProperty;

@ApplicationScoped
public class ActiveMqConnectionFactoryProducer {

    @ConfigProperty(name = "activemq.broker.url", defaultValue = "tcp://localhost:61616")
    String brokerUrl;

    @ConfigProperty(name = "activemq.username", defaultValue = "admin")
    String username;

    @ConfigProperty(name = "activemq.password", defaultValue = "admin")
    String password;

    @Produces
    @ApplicationScoped
    public ConnectionFactory createConnectionFactory() {
        ActiveMQConnectionFactory factory = new ActiveMQConnectionFactory();
        factory.setBrokerURL(brokerUrl);
        factory.setUserName(username);
        factory.setPassword(password);
        return factory;
    }
}
