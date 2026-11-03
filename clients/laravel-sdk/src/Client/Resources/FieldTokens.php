<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Model\FieldTokenIntrospectRequest;
use FlowCatalyst\Generated\Model\FieldTokenIntrospectResponse;

/**
 * Online check of a field access token (`POST /api/field-tokens/introspect`;
 * needs `platform:iam:field-token:introspect`). Use it when the offline
 * {@see \FlowCatalyst\Auth\Field\FieldTokenVerifier} is not enough and the
 * session must still be live. `active` is false when the token is unusable
 * (bad signature, expired, revoked session, suspended worker or device, `pcr`).
 */
class FieldTokens
{
    public function __construct(private readonly FlowCatalystClient $client) {}

    public function introspect(string $token): ?FieldTokenIntrospectResponse
    {
        return $this->client->generated()->introspectFieldToken((new FieldTokenIntrospectRequest())->setToken($token));
    }
}
