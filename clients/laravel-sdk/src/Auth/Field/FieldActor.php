<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Field;

/**
 * Who is signed in as the worker (the RFC 8693 `act` claim of a field token):
 * `type` is `field_worker` for a supervisor or `principal` for a platform user.
 */
final class FieldActor
{
    public function __construct(
        public readonly string $id,
        public readonly string $type,
        public readonly string $name,
    ) {}
}
