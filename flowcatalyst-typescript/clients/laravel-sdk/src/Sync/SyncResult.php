<?php

declare(strict_types=1);

namespace FlowCatalyst\Sync;

/**
 * Result of a sync operation for a single application.
 */
final class SyncResult
{
    /**
     * @param string $applicationCode The application that was synced
     * @param array{created: int, updated: int, deleted: int, error?: string} $roles Role sync results
     * @param array{created: int, updated: int, deleted: int, error?: string} $eventTypes Event type sync results
     * @param array{created: int, updated: int, deleted: int, error?: string} $subscriptions Subscription sync results
     * @param array{created: int, updated: int, deleted: int, error?: string} $dispatchPools Dispatch pool sync results
     */
    public function __construct(
        public readonly string $applicationCode,
        public readonly array $roles = ['created' => 0, 'updated' => 0, 'deleted' => 0],
        public readonly array $eventTypes = ['created' => 0, 'updated' => 0, 'deleted' => 0],
        public readonly array $subscriptions = ['created' => 0, 'updated' => 0, 'deleted' => 0],
        public readonly array $dispatchPools = ['created' => 0, 'updated' => 0, 'deleted' => 0],
    ) {}

    /**
     * Check if any roles were synced.
     */
    public function hasRoleChanges(): bool
    {
        return ($this->roles['created'] ?? 0) > 0
            || ($this->roles['updated'] ?? 0) > 0
            || ($this->roles['deleted'] ?? 0) > 0;
    }

    /**
     * Check if any event types were synced.
     */
    public function hasEventTypeChanges(): bool
    {
        return ($this->eventTypes['created'] ?? 0) > 0
            || ($this->eventTypes['updated'] ?? 0) > 0
            || ($this->eventTypes['deleted'] ?? 0) > 0;
    }

    /**
     * Check if any subscriptions were synced.
     */
    public function hasSubscriptionChanges(): bool
    {
        return ($this->subscriptions['created'] ?? 0) > 0
            || ($this->subscriptions['updated'] ?? 0) > 0
            || ($this->subscriptions['deleted'] ?? 0) > 0;
    }

    /**
     * Check if any dispatch pools were synced.
     */
    public function hasDispatchPoolChanges(): bool
    {
        return ($this->dispatchPools['created'] ?? 0) > 0
            || ($this->dispatchPools['updated'] ?? 0) > 0
            || ($this->dispatchPools['deleted'] ?? 0) > 0;
    }

    /**
     * Check if any changes were made.
     */
    public function hasChanges(): bool
    {
        return $this->hasRoleChanges()
            || $this->hasEventTypeChanges()
            || $this->hasSubscriptionChanges()
            || $this->hasDispatchPoolChanges();
    }

    /**
     * Check if there were any errors.
     */
    public function hasErrors(): bool
    {
        return isset($this->roles['error'])
            || isset($this->eventTypes['error'])
            || isset($this->subscriptions['error'])
            || isset($this->dispatchPools['error']);
    }

    /**
     * Get all error messages.
     *
     * @return string[]
     */
    public function getErrors(): array
    {
        $errors = [];

        if (isset($this->roles['error'])) {
            $errors['roles'] = $this->roles['error'];
        }

        if (isset($this->eventTypes['error'])) {
            $errors['eventTypes'] = $this->eventTypes['error'];
        }

        if (isset($this->subscriptions['error'])) {
            $errors['subscriptions'] = $this->subscriptions['error'];
        }

        if (isset($this->dispatchPools['error'])) {
            $errors['dispatchPools'] = $this->dispatchPools['error'];
        }

        return $errors;
    }

    /**
     * Get total counts across all definition types.
     *
     * @return array{created: int, updated: int, deleted: int}
     */
    public function getTotals(): array
    {
        return [
            'created' => ($this->roles['created'] ?? 0)
                + ($this->eventTypes['created'] ?? 0)
                + ($this->subscriptions['created'] ?? 0)
                + ($this->dispatchPools['created'] ?? 0),
            'updated' => ($this->roles['updated'] ?? 0)
                + ($this->eventTypes['updated'] ?? 0)
                + ($this->subscriptions['updated'] ?? 0)
                + ($this->dispatchPools['updated'] ?? 0),
            'deleted' => ($this->roles['deleted'] ?? 0)
                + ($this->eventTypes['deleted'] ?? 0)
                + ($this->subscriptions['deleted'] ?? 0)
                + ($this->dispatchPools['deleted'] ?? 0),
        ];
    }

    /**
     * Create an empty result for an application.
     */
    public static function empty(string $applicationCode): self
    {
        return new self($applicationCode);
    }
}
