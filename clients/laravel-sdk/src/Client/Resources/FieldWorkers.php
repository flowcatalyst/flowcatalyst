<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Model\StatusChangeResponse;
use FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest;
use FlowCatalyst\Generated\Model\FieldWorkerListResponse;
use FlowCatalyst\Generated\Model\FieldWorkerResponse;
use FlowCatalyst\Generated\Model\FieldWorkerSignInListResponse;
use FlowCatalyst\Generated\Model\GrantFieldWorkerAppRequest;
use FlowCatalyst\Generated\Model\ResetFieldWorkerPinResponse;
use FlowCatalyst\Generated\Model\ResetFieldWorkerPinRequest;
use FlowCatalyst\Generated\Model\UpdateFieldWorkerRequest;

/**
 * Field workers: the admin API (`/api/field-workers`). Thin wrappers over the
 * generated client; needs the `platform:iam:field-worker:view|manage`
 * permissions. PINs are never handled by app backends except a supervisor
 * setting or resetting one here.
 */
class FieldWorkers
{
    public function __construct(private readonly FlowCatalystClient $client) {}

    /** Idempotent create-or-get by (clientId, workerCode); appCodes are granted additively. */
    public function ensure(EnsureFieldWorkerRequest $request): ?FieldWorkerResponse
    {
        return $this->client->generated()->ensureFieldWorker($request);
    }

    /**
     * @param array{q?: string, appCode?: string, status?: string, page?: int, pageSize?: int} $filters
     */
    public function list(string $clientId, array $filters = []): ?FieldWorkerListResponse
    {
        return $this->client->generated()->listFieldWorkers(['clientId' => $clientId] + $filters);
    }

    public function get(string $id): ?FieldWorkerResponse
    {
        return $this->client->generated()->getFieldWorker($id);
    }

    public function update(string $id, UpdateFieldWorkerRequest $request): ?FieldWorkerResponse
    {
        return $this->client->generated()->updateFieldWorker($id, $request);
    }

    public function delete(string $id): ?StatusChangeResponse
    {
        return $this->client->generated()->deleteFieldWorker($id);
    }

    /** Omit $pin for a random temporary PIN, returned once in the response. */
    public function resetPin(string $id, ?string $pin = null): ?ResetFieldWorkerPinResponse
    {
        $body = new ResetFieldWorkerPinRequest();
        if ($pin !== null) {
            $body->setPin($pin);
        }
        return $this->client->generated()->resetFieldWorkerPin($id, $body);
    }

    public function unlock(string $id): ?StatusChangeResponse
    {
        return $this->client->generated()->unlockFieldWorker($id);
    }

    public function suspend(string $id): ?StatusChangeResponse
    {
        return $this->client->generated()->suspendFieldWorker($id);
    }

    public function activate(string $id): ?StatusChangeResponse
    {
        return $this->client->generated()->activateFieldWorker($id);
    }

    public function grantApp(string $id, string $appCode): ?StatusChangeResponse
    {
        return $this->client->generated()->grantFieldWorkerApp($id, (new GrantFieldWorkerAppRequest())->setAppCode($appCode));
    }

    public function revokeApp(string $id, string $appCode): ?StatusChangeResponse
    {
        return $this->client->generated()->revokeFieldWorkerApp($id, $appCode);
    }

    /** Recent sign-ins (page is 0-based). */
    public function signIns(string $id, ?int $page = null, ?int $pageSize = null): ?FieldWorkerSignInListResponse
    {
        $query = array_filter(['page' => $page, 'pageSize' => $pageSize], static fn ($v) => $v !== null);
        return $this->client->generated()->listFieldWorkerSignIns($id, $query);
    }
}
