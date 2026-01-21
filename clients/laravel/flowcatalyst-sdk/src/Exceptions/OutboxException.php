<?php

declare(strict_types=1);

namespace FlowCatalyst\Exceptions;

class OutboxException extends FlowCatalystException
{
    /**
     * Create an exception for missing tenant ID.
     */
    public static function missingTenantId(): static
    {
        return new static('Tenant ID is required. Please set FLOWCATALYST_TENANT_ID in your environment.');
    }

    /**
     * Create an exception for outbox not enabled.
     */
    public static function notEnabled(): static
    {
        return new static('Outbox is not enabled. Set FLOWCATALYST_OUTBOX_ENABLED=true in your environment.');
    }

    /**
     * Create an exception for driver not found.
     */
    public static function driverNotFound(string $driver): static
    {
        return new static("Outbox driver '{$driver}' is not supported. Supported drivers: database, mongodb.");
    }

    /**
     * Create an exception for insertion failure.
     */
    public static function insertFailed(string $reason): static
    {
        return new static("Failed to insert outbox message: {$reason}");
    }

    /**
     * Create an exception for MongoDB not installed.
     */
    public static function mongoDbNotInstalled(): static
    {
        return new static('MongoDB driver requires mongodb/laravel-mongodb package. Run: composer require mongodb/laravel-mongodb');
    }
}
