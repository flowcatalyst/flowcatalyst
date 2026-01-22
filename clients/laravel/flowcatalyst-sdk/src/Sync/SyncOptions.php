<?php

declare(strict_types=1);

namespace FlowCatalyst\Sync;

/**
 * Options for the sync operation.
 */
final class SyncOptions
{
    public function __construct(
        /**
         * Remove definitions from the platform that are not in the local set.
         * Only applies to API-sourced definitions (won't remove UI-created ones).
         */
        public readonly bool $removeUnlisted = false,

        /**
         * Whether to sync roles.
         */
        public readonly bool $syncRoles = true,

        /**
         * Whether to sync event types.
         */
        public readonly bool $syncEventTypes = true,

        /**
         * Whether to sync subscriptions.
         */
        public readonly bool $syncSubscriptions = true,
    ) {}

    /**
     * Create default options.
     */
    public static function defaults(): self
    {
        return new self();
    }

    /**
     * Create options that remove unlisted definitions.
     */
    public static function withRemoveUnlisted(): self
    {
        return new self(removeUnlisted: true);
    }

    /**
     * Create options that only sync roles.
     */
    public static function rolesOnly(): self
    {
        return new self(
            syncRoles: true,
            syncEventTypes: false,
            syncSubscriptions: false,
        );
    }

    /**
     * Create options that only sync event types.
     */
    public static function eventTypesOnly(): self
    {
        return new self(
            syncRoles: false,
            syncEventTypes: true,
            syncSubscriptions: false,
        );
    }

    /**
     * Create options that only sync subscriptions.
     */
    public static function subscriptionsOnly(): self
    {
        return new self(
            syncRoles: false,
            syncEventTypes: false,
            syncSubscriptions: true,
        );
    }

    /**
     * Create a copy with removeUnlisted enabled.
     */
    public function withRemoveUnlistedEnabled(): self
    {
        return new self(
            removeUnlisted: true,
            syncRoles: $this->syncRoles,
            syncEventTypes: $this->syncEventTypes,
            syncSubscriptions: $this->syncSubscriptions,
        );
    }
}
