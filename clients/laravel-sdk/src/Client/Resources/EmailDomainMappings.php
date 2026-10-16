<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\DTOs\EmailDomainMapping;

/**
 * Email-domain mappings (read-only lookup). Requires the platform's
 * email-domain-mapping read permission.
 */
class EmailDomainMappings
{
    public function __construct(
        private readonly FlowCatalystClient $client
    ) {}

    /**
     * Resolve an email domain (e.g. `example.com`) to its mapping. A domain
     * with no mapping is a 404 FlowCatalystException.
     */
    public function getByDomain(string $domain): EmailDomainMapping
    {
        return EmailDomainMapping::fromArray($this->getByDomainRaw($domain));
    }

    /**
     * Same lookup, returning the platform's decoded body unchanged.
     *
     * @return array<string, mixed>
     */
    public function getByDomainRaw(string $domain): array
    {
        return $this->client->request(
            'GET',
            '/api/email-domain-mappings/by-domain/' . rawurlencode($domain),
        );
    }
}
