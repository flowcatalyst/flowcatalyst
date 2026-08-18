<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Requests;

/**
 * Payload for POST /api/principals/users.
 *
 * `password` is optional: omit it for OIDC-authenticated users. When
 * `enforcePasswordComplexity` is false the platform skips its complexity
 * rules and only enforces a 2-character minimum — intended for apps that
 * enforce their own password policy.
 *
 * `scope` is optional and defaults to CLIENT. The platform never upgrades
 * it from the email domain: pass 'ANCHOR' or 'PARTNER' explicitly, and the
 * domain setup must back it (registered anchor domain / PARTNER
 * email-domain mapping) or the create is rejected.
 *
 * `clientId` accepts either the FlowCatalyst client id (`clt_…`) or the
 * client's identifier slug — the platform resolves both, so you can pass
 * your tenant code directly instead of listing clients to find the id.
 */
final class CreateUserRequest
{
    public function __construct(
        public readonly string $email,
        public readonly string $name,
        public readonly ?string $password = null,
        public readonly ?string $clientId = null,
        public readonly ?bool $enforcePasswordComplexity = null,
        public readonly ?string $scope = null,
    ) {}

    /**
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        $payload = [
            'email' => $this->email,
            'name' => $this->name,
        ];
        if ($this->password !== null) {
            $payload['password'] = $this->password;
        }
        if ($this->clientId !== null) {
            $payload['clientId'] = $this->clientId;
        }
        if ($this->enforcePasswordComplexity !== null) {
            $payload['enforcePasswordComplexity'] = $this->enforcePasswordComplexity;
        }
        if ($this->scope !== null) {
            $payload['scope'] = $this->scope;
        }
        return $payload;
    }
}
