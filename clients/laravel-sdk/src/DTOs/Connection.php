<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

use FlowCatalyst\Enums\ConnectionStatus;

class Connection
{
    public function __construct(
        public readonly string $id,
        public readonly string $code,
        public readonly string $name,
        public readonly ?string $description,
        public readonly string $endpoint,
        public readonly ?string $externalId,
        public readonly ConnectionStatus $status,
        public readonly string $serviceAccountId,
        public readonly ?string $clientId,
        public readonly ?string $clientIdentifier,
        public readonly string $createdAt,
        public readonly string $updatedAt,
    ) {}

    public static function fromArray(array $data): self
    {
        return new self(
            id: $data['id'],
            code: $data['code'],
            name: $data['name'],
            description: $data['description'] ?? null,
            endpoint: $data['endpoint'],
            externalId: $data['externalId'] ?? null,
            status: ConnectionStatus::tryFrom($data['status'] ?? 'ACTIVE') ?? ConnectionStatus::ACTIVE,
            serviceAccountId: $data['serviceAccountId'],
            clientId: $data['clientId'] ?? null,
            clientIdentifier: $data['clientIdentifier'] ?? null,
            createdAt: $data['createdAt'] ?? '',
            updatedAt: $data['updatedAt'] ?? '',
        );
    }

    public function toArray(): array
    {
        return [
            'id' => $this->id,
            'code' => $this->code,
            'name' => $this->name,
            'description' => $this->description,
            'endpoint' => $this->endpoint,
            'externalId' => $this->externalId,
            'status' => $this->status->value,
            'serviceAccountId' => $this->serviceAccountId,
            'clientId' => $this->clientId,
            'clientIdentifier' => $this->clientIdentifier,
            'createdAt' => $this->createdAt,
            'updatedAt' => $this->updatedAt,
        ];
    }
}
