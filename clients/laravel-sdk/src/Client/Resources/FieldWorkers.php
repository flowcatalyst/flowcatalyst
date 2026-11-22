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
use FlowCatalyst\DTOs\FieldWorkerPhotoImage;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Generated\Client as GeneratedClient;
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

    /**
     * Idempotent create-or-get by (clientId, workerCode); appCodes are granted additively. Without a
     * worker code the platform generates one (three letters, then the client policy's
     * workerCodeDigits digits, e.g. KTM482) and the response carries it; an idempotency key (only
     * without a code) makes a repeat answer the worker the first call made instead of a second one.
     * The generated client does not resend a request whose answer was lost.
     */
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

    /**
     * Set or replace the worker's photo. `$bytes` is the picture itself (JPEG, PNG or WebP): the
     * platform decides the type from the bytes, scales it to 512 px (full) and 128 px (thumb) and
     * keeps nothing else of the upload, metadata included. The generated client sends it as
     * `application/octet-stream`, which the platform accepts for any of the three. Returns the
     * refreshed worker. A refusal carries the platform's code in the generated `ErrorModel`'s
     * `error` (PHOTO_EMPTY, PHOTO_TOO_LARGE 413, PHOTO_UNSUPPORTED_TYPE 415, PHOTO_TYPE_MISMATCH,
     * PHOTO_DIMENSIONS_TOO_LARGE, PHOTO_UNREADABLE, PHOTO_BUSY 503, PHOTOS_DISABLED_FOR_CLIENT 409,
     * PHOTOS_DISABLED 404).
     */
    public function setPhoto(string $id, string $bytes): ?FieldWorkerResponse
    {
        return $this->client->generated()->setFieldWorkerPhoto($id, $bytes);
    }

    /**
     * Read the worker's photo as bytes (`$size` is `thumb`, the default, or `full`). Pass the `etag`
     * of an earlier read as `$ifNoneMatch`: while the photo is unchanged the answer is a
     * {@see FieldWorkerPhotoImage} with `notModified` true and no bytes. A worker with no photo
     * (or a client with photos off) throws a {@see FlowCatalystException} with code 404 and context
     * `['code' => 'PHOTO_NOT_FOUND']`. The generated client drops an image body (it declares none), so
     * this reads the response itself.
     */
    public function getPhoto(string $id, string $size = 'thumb', ?string $ifNoneMatch = null): FieldWorkerPhotoImage
    {
        $headers = $ifNoneMatch !== null && $ifNoneMatch !== '' ? ['If-None-Match' => $ifNoneMatch] : [];
        $response = $this->client->generated()->getFieldWorkerPhoto($id, ['size' => $size], $headers, GeneratedClient::FETCH_RESPONSE);
        $status = $response->getStatusCode();
        $etag = $response->getHeaderLine('ETag') ?: null;
        if ($status === 304) {
            return new FieldWorkerPhotoImage(null, null, $etag ?? $ifNoneMatch, true);
        }
        if ($status >= 200 && $status < 300) {
            return new FieldWorkerPhotoImage((string) $response->getBody(), $response->getHeaderLine('Content-Type') ?: null, $etag, false);
        }
        $body = json_decode((string) $response->getBody(), true);
        $code = is_array($body) ? ($body['error'] ?? $body['code'] ?? null) : null;
        $message = is_array($body) && isset($body['message']) ? (string) $body['message'] : 'HTTP ' . $status;
        throw new FlowCatalystException($message, $status, null, ['code' => $code]);
    }

    /** Remove the worker's photo. Not an error when there is none. Returns the refreshed worker. */
    public function removePhoto(string $id): ?FieldWorkerResponse
    {
        return $this->client->generated()->removeFieldWorkerPhoto($id);
    }
}
