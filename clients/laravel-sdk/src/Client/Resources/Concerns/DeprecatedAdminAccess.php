<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources\Concerns;

use FlowCatalyst\Support\Deprecations;

/**
 * Lets a resource class serve both the legacy accessor (`$client->principals()`)
 * and the Admin facade (`$client->admin()->principals()`) with identical
 * behaviour. Only the legacy path emits the once-per-process deprecation notice
 * for methods that moved to {@see \FlowCatalyst\Client\Admin}.
 *
 * The using class must declare `private const ADMIN_ACCESSOR = '<name>'`.
 */
trait DeprecatedAdminAccess
{
    private bool $viaAdmin = false;

    /**
     * Copy of this resource that does not emit deprecation notices.
     *
     * @internal Used by {@see \FlowCatalyst\Client\Admin}.
     */
    public function viaAdmin(): static
    {
        $copy = clone $this;
        $copy->viaAdmin = true;

        return $copy;
    }

    private function deprecatedAdminMethod(string $method): void
    {
        if ($this->viaAdmin) {
            return;
        }

        $class = static::class;
        $accessor = self::ADMIN_ACCESSOR;

        Deprecations::once(
            "{$class}::{$method}",
            "{$class}::{$method}() is deprecated and will be removed in a future breaking release; "
            . "use \$client->admin()->{$accessor}()->{$method}() instead."
        );
    }
}
