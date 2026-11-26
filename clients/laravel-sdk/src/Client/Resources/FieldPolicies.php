<?php

declare(strict_types=1);

namespace FlowCatalyst\Client\Resources;

use FlowCatalyst\Client\FlowCatalystClient;
use FlowCatalyst\Generated\Model\DeleteClientPhotosResponse;
use FlowCatalyst\Generated\Model\FieldPolicyResponse;
use FlowCatalyst\Exceptions\FlowCatalystException;
use FlowCatalyst\Generated\Model\PutFieldPolicyRequest;

/**
 * Per-client field-identity policy (PIN rules, lockout, idle and shift
 * timeouts). Needs `platform:iam:field-device:view|manage`.
 */
class FieldPolicies
{
    /** The settings of a policy, as the suffix of their getter and setter. */
    private const SETTINGS = [
        'PinMinLength', 'SoftLockThreshold', 'SoftLockSecs', 'HardLockThreshold', 'SharedIdleSecs',
        'PersonalIdleSecs', 'ShiftMaxSecs', 'RequireAttestation', 'PinReauthDays', 'RequireEnrolledDevice',
        'UnenrolledIdleSecs', 'BadgeOnlyLogin', 'ImpersonationMaxSecs', 'WorkerCodeDigits', 'PhotosEnabled',
        'WorkerMaySetOwnPhoto', 'UnlockKeysEnabled', 'UnlockKeyAttestation',
    ];

    public function __construct(private readonly FlowCatalystClient $client) {}

    public function get(string $clientId): ?FieldPolicyResponse
    {
        return $this->client->generated()->getFieldPolicy($clientId);
    }

    /**
     * A PUT REPLACES the policy: a field left out takes its default (`photosEnabled` true,
     * `workerMaySetOwnPhoto` false, `pinReauthDays` 0, `unlockKeysEnabled` false,
     * `unlockKeyAttestation` NONE), so `get` the policy and send every field back to change one:
     * use {@see update()}, or start from {@see toRequest()}.
     *
     * A request that does not set `unlockKeysEnabled` switches unlock keys off for the client,
     * revokes all its keys and signs those workers out. This method sends exactly what it is given.
     */
    public function put(string $clientId, PutFieldPolicyRequest $request): ?FieldPolicyResponse
    {
        return $this->client->generated()->putFieldPolicy($clientId, $request);
    }

    /**
     * The PUT request that stores `$policy` again unchanged: every setting the platform returned is
     * set on it (the unlock-key settings and `pinReauthDays` 0 included); the response-only members
     * (`clientId`, `isDefault`, `updatedAt`) are not.
     */
    public static function toRequest(FieldPolicyResponse $policy): PutFieldPolicyRequest
    {
        $request = new PutFieldPolicyRequest();
        foreach (self::SETTINGS as $name) {
            $value = $policy->{'get' . $name}();
            if ($value !== null) {
                $request->{'set' . $name}($value);
            }
        }
        return $request;
    }

    /**
     * Read the policy, let `$change` alter the request that would store it unchanged, and write the
     * result: every setting `$change` does not touch keeps its stored value, settings this SDK
     * version does not know about excepted (it sends back what its generated model carries). The read
     * and the write are two calls: a policy edited by someone else in between is overwritten with what
     * was read.
     *
     * @param callable(PutFieldPolicyRequest): void $change
     */
    public function update(string $clientId, callable $change): ?FieldPolicyResponse
    {
        $current = $this->get($clientId);
        if ($current === null) {
            throw new FlowCatalystException('The field policy of ' . $clientId . ' could not be read.');
        }
        $request = self::toRequest($current);
        $change($request);
        return $this->put($clientId, $request);
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
