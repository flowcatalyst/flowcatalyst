-- V25: Create subscription_configs table for custom config entries

CREATE TABLE subscription_configs (
    id BIGSERIAL PRIMARY KEY,
    subscription_id VARCHAR(17) NOT NULL,
    config_key VARCHAR(100) NOT NULL,
    config_value VARCHAR(1000)
);

CREATE INDEX idx_subscription_configs_subscription_id ON subscription_configs(subscription_id);
CREATE UNIQUE INDEX idx_subscription_configs_unique ON subscription_configs(subscription_id, config_key);
