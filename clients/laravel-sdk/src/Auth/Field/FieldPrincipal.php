<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Field;

use Illuminate\Contracts\Auth\Authenticatable;

/**
 * A verified field worker on a verified key. Built only by
 * {@see FieldTokenVerifier}. Implements Authenticatable (identifier = the
 * worker id) so it can be the user of the `field` guard.
 *
 * When `$actor` is set, someone else is signed in as the worker: every other
 * property is still the WORKER's, so an app's audit trail records the worker as
 * the subject and `$actor` as the operator.
 */
final class FieldPrincipal implements Authenticatable
{
    /**
     * @param list<string> $amr
     */
    public function __construct(
        public readonly string $workerId,
        public readonly string $clientId,
        public readonly string $clientIdentifier,
        public readonly string $workerCode,
        public readonly string $name,
        public readonly string $appCode,
        /** Empty when the key is not an enrolled device (`$deviceMode` is then `UNENROLLED`). */
        public readonly string $deviceId,
        public readonly string $deviceMode,
        /** Login family (sid). */
        public readonly string $sessionId,
        public readonly array $amr,
        public readonly ?\DateTimeImmutable $authTime,
        public readonly \DateTimeImmutable $expiresAt,
        /** Thumbprint (RFC 7638) of the device key the request was proven with. */
        public readonly string $jkt,
        public readonly string $tokenId,
        /** Set when someone other than the worker is signed in as them (the `act` claim). */
        public readonly ?FieldActor $actor = null,
        /** True for a test identity. */
        public readonly bool $test = false,
    ) {}

    public function getAuthIdentifierName(): string
    {
        return 'workerId';
    }

    public function getAuthIdentifier(): string
    {
        return $this->workerId;
    }

    public function getAuthPasswordName(): string
    {
        return '';
    }

    public function getAuthPassword(): string
    {
        return '';
    }

    public function getRememberToken(): ?string
    {
        return null;
    }

    public function setRememberToken($value): void {}

    public function getRememberTokenName(): string
    {
        return '';
    }
}
