<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Model\StatusChangeResponse;
use FlowCatalyst\Generated\Model\BulkIssueFieldBadgesRequest;
use FlowCatalyst\Generated\Model\BulkIssueFieldBadgesResponse;
use FlowCatalyst\Generated\Model\CreateFieldSignInCodeRequest;
use FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest;
use FlowCatalyst\Generated\Model\FieldBadgeIssuedResponse;
use FlowCatalyst\Generated\Model\FieldBadgeListResponse;
use FlowCatalyst\Generated\Model\FieldBadgeResponse;
use FlowCatalyst\Generated\Model\FieldSignInCodeResponse;
use FlowCatalyst\Generated\Model\IssueFieldBadgeRequest;
use FlowCatalyst\Generated\Model\FieldWorkerListResponse;
use FlowCatalyst\Generated\Model\FieldWorkerResponse;
use FlowCatalyst\Generated\Model\FieldWorkerSignInListResponse;
use FlowCatalyst\Generated\Model\GrantFieldWorkerAppRequest;
use FlowCatalyst\Generated\Model\LinkFieldWorkerRequest;
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
     * `isTest` limits to test identities (true) or real workers (false); `linkedPrincipalId` limits to the
     * worker linked to that platform user.
     *
     * @param array{q?: string, appCode?: string, status?: string, isTest?: bool|string, linkedPrincipalId?: string, page?: int, pageSize?: int} $filters
     */
    public function list(string $clientId, array $filters = []): ?FieldWorkerListResponse
    {
        if (isset($filters['isTest']) && is_bool($filters['isTest'])) {
            $filters['isTest'] = $filters['isTest'] ? 'true' : 'false';
        }
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

    /**
     * Link the worker to a platform user, for audit correlation only (the worker's tokens then carry
     * `linked_principal_id`; it grants nothing). 422 INVALID_LINK when the user cannot be linked, 409
     * LINK_EXISTS when another worker of the client already holds the link. The response carries `linkedPrincipal`.
     */
    public function link(string $id, string $principalId): ?FieldWorkerResponse
    {
        return $this->client->generated()->linkFieldWorker($id, (new LinkFieldWorkerRequest())->setPrincipalId($principalId));
    }

    /** Remove the worker's link to a platform user (idempotent). */
    public function unlink(string $id): ?FieldWorkerResponse
    {
        return $this->client->generated()->unlinkFieldWorker($id);
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

    /**
     * Issue a badge (replaces the worker's ACTIVE badge). The response's `token`
     * is the secret to print as a QR code; it is shown once.
     */
    public function issueBadge(string $id, ?string $label = null): ?FieldBadgeIssuedResponse
    {
        $body = new IssueFieldBadgeRequest();
        if ($label !== null) {
            $body->setLabel($label);
        }
        return $this->client->generated()->issueFieldWorkerBadge($id, $body);
    }

    /** The worker's badges (tokens are never returned). */
    public function listBadges(string $id): ?FieldBadgeListResponse
    {
        return $this->client->generated()->listFieldWorkerBadges($id);
    }

    public function revokeBadge(string $id, string $badgeId): ?FieldBadgeResponse
    {
        return $this->client->generated()->revokeFieldWorkerBadge($id, $badgeId);
    }

    /**
     * Issue a badge for each of 1 to 200 workers in one transaction (all or
     * nothing). Each result carries the token, shown once.
     *
     * @param list<string> $workerIds
     */
    public function bulkIssueBadges(array $workerIds): ?BulkIssueFieldBadgesResponse
    {
        return $this->client->generated()->issueFieldWorkerBadges((new BulkIssueFieldBadgesRequest())->setWorkerIds($workerIds));
    }

    /**
     * Mint a single-use code a console admin redeems at the platform's field
     * sign-in to start an impersonated session as the worker.
     */
    public function createSignInCode(string $id, string $appCode): ?FieldSignInCodeResponse
    {
        return $this->client->generated()->createFieldWorkerSignInCode($id, (new CreateFieldSignInCodeRequest())->setAppCode($appCode));
    }
}
