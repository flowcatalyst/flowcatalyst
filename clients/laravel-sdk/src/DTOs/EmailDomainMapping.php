<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

/**
 * An email-domain mapping as returned by
 * GET /api/email-domain-mappings/by-domain/{domain}.
 */
class EmailDomainMapping
{
    /**
     * @param string[] $additionalClientIds
     * @param string[] $grantedClientIds
     * @param string[] $allowed2faMethods
     */
    public function __construct(
        public readonly string $id,
        public readonly string $emailDomain,
        public readonly string $identityProviderId,
        public readonly ?string $identityProviderName,
        public readonly string $scopeType,
        public readonly ?string $primaryClientId,
        public readonly array $additionalClientIds,
        public readonly array $grantedClientIds,
        public readonly ?string $requiredOidcTenantId,
        public readonly bool $require2fa,
        public readonly array $allowed2faMethods,
        public readonly bool $rememberDeviceEnabled,
        public readonly int $rememberDeviceDays,
        public readonly string $createdAt,
        public readonly string $updatedAt,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: (string) ($data['id'] ?? ''),
            emailDomain: (string) ($data['emailDomain'] ?? ''),
            identityProviderId: (string) ($data['identityProviderId'] ?? ''),
            identityProviderName: isset($data['identityProviderName']) ? (string) $data['identityProviderName'] : null,
            scopeType: (string) ($data['scopeType'] ?? ''),
            primaryClientId: isset($data['primaryClientId']) ? (string) $data['primaryClientId'] : null,
            additionalClientIds: $data['additionalClientIds'] ?? [],
            grantedClientIds: $data['grantedClientIds'] ?? [],
            requiredOidcTenantId: isset($data['requiredOidcTenantId']) ? (string) $data['requiredOidcTenantId'] : null,
            require2fa: (bool) ($data['require2fa'] ?? false),
            allowed2faMethods: $data['allowed2faMethods'] ?? [],
            rememberDeviceEnabled: (bool) ($data['rememberDeviceEnabled'] ?? false),
            rememberDeviceDays: (int) ($data['rememberDeviceDays'] ?? 0),
            createdAt: (string) ($data['createdAt'] ?? ''),
            updatedAt: (string) ($data['updatedAt'] ?? ''),
        );
    }
}
