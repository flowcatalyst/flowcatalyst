<?php

declare(strict_types=1);

namespace FlowCatalyst\Sync;

/**
 * Container for definitions to sync for a single FlowCatalyst application.
 *
 * This class allows you to build a complete set of definitions for one
 * application and sync them all at once. Definitions can be provided as
 * DTOs (RoleDefinition, EventTypeDefinition, SubscriptionDefinition) or
 * as plain arrays.
 *
 * Example usage:
 *
 * ```php
 * // Using DTOs
 * $definitions = SyncDefinitionSet::forApplication('my-app')
 *     ->withRoles([
 *         new RoleDefinition('admin', 'Administrator', permissions: ['user:manage']),
 *         new RoleDefinition('editor', 'Editor', permissions: ['content:edit']),
 *     ])
 *     ->withEventTypes([
 *         new EventTypeDefinition('user.created', 'User Created'),
 *     ]);
 *
 * // Using arrays
 * $definitions = SyncDefinitionSet::forApplication('my-app')
 *     ->withRoles([
 *         ['name' => 'admin', 'displayName' => 'Administrator'],
 *     ]);
 *
 * // Sync
 * $synchronizer->sync($definitions);
 * ```
 */
final class SyncDefinitionSet
{
    /** @var array<RoleDefinition|array<string, mixed>> */
    private array $roles = [];

    /** @var array<EventTypeDefinition|array<string, mixed>> */
    private array $eventTypes = [];

    /** @var array<SubscriptionDefinition|array<string, mixed>> */
    private array $subscriptions = [];

    /** @var array<DispatchPoolDefinition|array<string, mixed>> */
    private array $dispatchPools = [];

    /** @var array<PrincipalDefinition|array<string, mixed>> */
    private array $principals = [];

    public function __construct(
        public readonly string $applicationCode,
    ) {}

    /**
     * Create a new definition set for an application.
     */
    public static function forApplication(string $applicationCode): self
    {
        return new self($applicationCode);
    }

    /**
     * Add roles to the definition set.
     *
     * @param array<RoleDefinition|array<string, mixed>> $roles
     */
    public function withRoles(array $roles): self
    {
        $clone = clone $this;
        $clone->roles = $roles;
        return $clone;
    }

    /**
     * Add a single role to the definition set.
     *
     * @param RoleDefinition|array<string, mixed> $role
     */
    public function addRole(RoleDefinition|array $role): self
    {
        $clone = clone $this;
        $clone->roles = [...$this->roles, $role];
        return $clone;
    }

    /**
     * Add event types to the definition set.
     *
     * @param array<EventTypeDefinition|array<string, mixed>> $eventTypes
     */
    public function withEventTypes(array $eventTypes): self
    {
        $clone = clone $this;
        $clone->eventTypes = $eventTypes;
        return $clone;
    }

    /**
     * Add a single event type to the definition set.
     *
     * @param EventTypeDefinition|array<string, mixed> $eventType
     */
    public function addEventType(EventTypeDefinition|array $eventType): self
    {
        $clone = clone $this;
        $clone->eventTypes = [...$this->eventTypes, $eventType];
        return $clone;
    }

    /**
     * Add subscriptions to the definition set.
     *
     * @param array<SubscriptionDefinition|array<string, mixed>> $subscriptions
     */
    public function withSubscriptions(array $subscriptions): self
    {
        $clone = clone $this;
        $clone->subscriptions = $subscriptions;
        return $clone;
    }

    /**
     * Add a single subscription to the definition set.
     *
     * @param SubscriptionDefinition|array<string, mixed> $subscription
     */
    public function addSubscription(SubscriptionDefinition|array $subscription): self
    {
        $clone = clone $this;
        $clone->subscriptions = [...$this->subscriptions, $subscription];
        return $clone;
    }

    /**
     * Add dispatch pools to the definition set.
     *
     * @param array<DispatchPoolDefinition|array<string, mixed>> $dispatchPools
     */
    public function withDispatchPools(array $dispatchPools): self
    {
        $clone = clone $this;
        $clone->dispatchPools = $dispatchPools;
        return $clone;
    }

    /**
     * Add a single dispatch pool to the definition set.
     *
     * @param DispatchPoolDefinition|array<string, mixed> $dispatchPool
     */
    public function addDispatchPool(DispatchPoolDefinition|array $dispatchPool): self
    {
        $clone = clone $this;
        $clone->dispatchPools = [...$this->dispatchPools, $dispatchPool];
        return $clone;
    }

    /**
     * Get roles as arrays for the sync API.
     *
     * @return array<array<string, mixed>>
     */
    public function getRoles(): array
    {
        return array_map(
            fn($role) => $role instanceof RoleDefinition ? $role->toArray() : $role,
            $this->roles
        );
    }

    /**
     * Get event types as arrays for the sync API.
     *
     * @return array<array<string, mixed>>
     */
    public function getEventTypes(): array
    {
        return array_map(
            fn($et) => $et instanceof EventTypeDefinition ? $et->toArray() : $et,
            $this->eventTypes
        );
    }

    /**
     * Get subscriptions as arrays for the sync API.
     *
     * @return array<array<string, mixed>>
     */
    public function getSubscriptions(): array
    {
        return array_map(
            fn($sub) => $sub instanceof SubscriptionDefinition ? $sub->toArray() : $sub,
            $this->subscriptions
        );
    }

    /**
     * Get dispatch pools as arrays for the sync API.
     *
     * @return array<array<string, mixed>>
     */
    public function getDispatchPools(): array
    {
        return array_map(
            fn($pool) => $pool instanceof DispatchPoolDefinition ? $pool->toArray() : $pool,
            $this->dispatchPools
        );
    }

    /**
     * Add principals to the definition set.
     *
     * @param array<PrincipalDefinition|array<string, mixed>> $principals
     */
    public function withPrincipals(array $principals): self
    {
        $clone = clone $this;
        $clone->principals = $principals;
        return $clone;
    }

    /**
     * Add a single principal to the definition set.
     *
     * @param PrincipalDefinition|array<string, mixed> $principal
     */
    public function addPrincipal(PrincipalDefinition|array $principal): self
    {
        $clone = clone $this;
        $clone->principals = [...$this->principals, $principal];
        return $clone;
    }

    /**
     * Get principals as arrays for the sync API.
     *
     * @return array<array<string, mixed>>
     */
    public function getPrincipals(): array
    {
        return array_map(
            fn($p) => $p instanceof PrincipalDefinition ? $p->toArray() : $p,
            $this->principals
        );
    }

    /**
     * Check if there are any principals to sync.
     */
    public function hasPrincipals(): bool
    {
        return !empty($this->principals);
    }

    /**
     * Check if there are any roles to sync.
     */
    public function hasRoles(): bool
    {
        return !empty($this->roles);
    }

    /**
     * Check if there are any event types to sync.
     */
    public function hasEventTypes(): bool
    {
        return !empty($this->eventTypes);
    }

    /**
     * Check if there are any subscriptions to sync.
     */
    public function hasSubscriptions(): bool
    {
        return !empty($this->subscriptions);
    }

    /**
     * Check if there are any dispatch pools to sync.
     */
    public function hasDispatchPools(): bool
    {
        return !empty($this->dispatchPools);
    }

    /**
     * Check if the definition set is empty.
     */
    public function isEmpty(): bool
    {
        return !$this->hasRoles() && !$this->hasEventTypes() && !$this->hasSubscriptions() && !$this->hasDispatchPools() && !$this->hasPrincipals();
    }

    /**
     * Create from the scanned definitions repository format.
     *
     * @param string $applicationCode
     * @param array{roles?: array<array<string, mixed>>, eventTypes?: array<array<string, mixed>>, subscriptions?: array<array<string, mixed>>, dispatchPools?: array<array<string, mixed>>, principals?: array<array<string, mixed>>} $data
     */
    public static function fromScannedDefinitions(string $applicationCode, array $data): self
    {
        $set = new self($applicationCode);

        // Remove internal _class field from scanned definitions
        $set->roles = array_map(function ($role) {
            unset($role['_class']);
            return $role;
        }, $data['roles'] ?? []);

        $set->eventTypes = array_map(function ($et) {
            unset($et['_class']);
            return $et;
        }, $data['eventTypes'] ?? []);

        $set->subscriptions = array_map(function ($sub) {
            unset($sub['_class']);
            return $sub;
        }, $data['subscriptions'] ?? []);

        $set->dispatchPools = array_map(function ($pool) {
            unset($pool['_class']);
            return $pool;
        }, $data['dispatchPools'] ?? []);

        $set->principals = array_map(function ($principal) {
            unset($principal['_class']);
            return $principal;
        }, $data['principals'] ?? []);

        return $set;
    }
}
