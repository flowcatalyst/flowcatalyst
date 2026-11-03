<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Model\StatusChangeResponse;
use FlowCatalyst\Generated\Model\CreateEnrolmentCodeRequest;
use FlowCatalyst\Generated\Model\FieldDeviceListResponse;
use FlowCatalyst\Generated\Model\FieldDeviceResponse;
use FlowCatalyst\Generated\Model\FieldEnrolmentCodeResponse;

/**
 * Field devices and enrolment codes: the admin API (`/api/field-devices`).
 * Thin wrappers over the generated client; needs the
 * `platform:iam:field-device:view|manage` permissions.
 */
class FieldDevices
{
    public function __construct(private readonly FlowCatalystClient $client) {}

    /** The code is shown once; `qrPayload` is `fcfield:1:<platform base URL>:<code>`. */
    public function createEnrolmentCode(CreateEnrolmentCodeRequest $request): ?FieldEnrolmentCodeResponse
    {
        return $this->client->generated()->createFieldEnrolmentCode($request);
    }

    /**
     * @param array{status?: string, mode?: string, page?: int, pageSize?: int} $filters
     */
    public function list(string $clientId, array $filters = []): ?FieldDeviceListResponse
    {
        return $this->client->generated()->listFieldDevices(['clientId' => $clientId] + $filters);
    }

    public function get(string $id): ?FieldDeviceResponse
    {
        return $this->client->generated()->getFieldDevice($id);
    }

    public function suspend(string $id): ?StatusChangeResponse
    {
        return $this->client->generated()->suspendFieldDevice($id);
    }

    public function activate(string $id): ?StatusChangeResponse
    {
        return $this->client->generated()->activateFieldDevice($id);
    }

    public function revoke(string $id): ?StatusChangeResponse
    {
        return $this->client->generated()->revokeFieldDevice($id);
    }
}
