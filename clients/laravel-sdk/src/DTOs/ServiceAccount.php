<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

/**
 * A service account — the non-human credential attached to an application, as
 * Applications::getServiceAccount answers it (the body of
 * `GET /api/service-accounts/{id}`; provisioning answers a
 * ProvisionedServiceAccount). `$id` is the account's own id (`sac_…`);
 * `$principalId` is its SERVICE principal (`prn_…`), which is what an
 * application's `serviceAccountId` holds; `$oauthClientId` is the public
 * OAuth client id it authenticates with. No secret is ever part of it.
 */
final class ServiceAccount
{
    public function __construct(
        public readonly string $id,
        public readonly string $code,
        public readonly string $name,
        public readonly bool $active,
        public readonly string $createdAt,
        public readonly ?string $description = null,
        public readonly ?string $applicationId = null,
        public readonly ?string $principalId = null,
        public readonly ?string $oauthClientId = null,
        public readonly ?string $authType = null,
        /** @var list<string> */
        public readonly array $clientIds = [],
        /** @var list<string> */
        public readonly array $roles = [],
        public readonly ?string $scope = null,
        public readonly ?string $lastUsedAt = null,
        public readonly ?string $updatedAt = null,
    ) {}

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        return new self(
            id: (string) $data['id'],
            code: (string) $data['code'],
            name: (string) $data['name'],
            active: (bool) ($data['active'] ?? true),
            createdAt: (string) ($data['createdAt'] ?? ''),
            description: isset($data['description']) ? (string) $data['description'] : null,
            applicationId: isset($data['applicationId']) ? (string) $data['applicationId'] : null,
            principalId: isset($data['principalId']) ? (string) $data['principalId'] : null,
            oauthClientId: isset($data['oauthClientId']) ? (string) $data['oauthClientId'] : null,
            authType: isset($data['authType']) ? (string) $data['authType'] : null,
            clientIds: array_values(array_map('strval', (array) ($data['clientIds'] ?? []))),
            roles: array_values(array_map('strval', (array) ($data['roles'] ?? []))),
            scope: isset($data['scope']) ? (string) $data['scope'] : null,
            lastUsedAt: isset($data['lastUsedAt']) ? (string) $data['lastUsedAt'] : null,
            updatedAt: isset($data['updatedAt']) ? (string) $data['updatedAt'] : null,
        );
    }

    /**
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        return [
            'id' => $this->id,
            'code' => $this->code,
            'name' => $this->name,
            'description' => $this->description,
            'active' => $this->active,
            'applicationId' => $this->applicationId,
            'principalId' => $this->principalId,
            'oauthClientId' => $this->oauthClientId,
            'authType' => $this->authType,
            'clientIds' => $this->clientIds,
            'roles' => $this->roles,
            'scope' => $this->scope,
            'lastUsedAt' => $this->lastUsedAt,
            'createdAt' => $this->createdAt,
            'updatedAt' => $this->updatedAt,
        ];
    }
}
