<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Model\FieldPolicyResponse;
use FlowCatalyst\Generated\Model\PutFieldPolicyRequest;

/**
 * Per-client field-identity policy (PIN rules, lockout, idle and shift
 * timeouts). Needs `platform:iam:field-device:view|manage`.
 */
class FieldPolicies
{
    public function __construct(private readonly FlowCatalystClient $client) {}

    public function get(string $clientId): ?FieldPolicyResponse
    {
        return $this->client->generated()->getFieldPolicy($clientId);
    }

    public function put(string $clientId, PutFieldPolicyRequest $request): ?FieldPolicyResponse
    {
        return $this->client->generated()->putFieldPolicy($clientId, $request);
    }
}
