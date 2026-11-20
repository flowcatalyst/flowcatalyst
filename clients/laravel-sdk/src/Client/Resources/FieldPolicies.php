<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Model\DeleteClientPhotosResponse;
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

    /**
     * A PUT replaces the policy: a field left out takes its default (`photosEnabled` true,
     * `workerMaySetOwnPhoto` false), so `get` the policy and send every field back to change one.
     */
    public function put(string $clientId, PutFieldPolicyRequest $request): ?FieldPolicyResponse
    {
        return $this->client->generated()->putFieldPolicy($clientId, $request);
    }

    /**
     * Delete every stored photo of the client's field workers, whatever the policy's
     * `photosEnabled` says (switching photos off only hides them). Answers how many were removed;
     * repeating it removes none. Needs `platform:iam:field-worker:manage` for the client; an
     * application-confined service account is refused (403 APPLICATION_FORBIDDEN): it removes its
     * own workers' photos one at a time. It cannot be undone.
     */
    public function deletePhotos(string $clientId): ?DeleteClientPhotosResponse
    {
        return $this->client->generated()->deleteClientFieldWorkerPhotos($clientId);
    }
}
