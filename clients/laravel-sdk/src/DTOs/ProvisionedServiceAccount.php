<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

use JsonSerializable;

/**
 * Answer to provisioning an application's service account
 * (POST /api/applications/{id}/provision-service-account): the account's
 * principal, its OAuth client and its webhook credentials.
 *
 * `$clientSecret`, `$webhookAuthToken` and `$webhookSigningSecret` are the
 * plaintext secrets, shown this once: the platform cannot show them again
 * (rotate with the service account's regenerate routes). They are sensitive:
 * do not log them. The DTO redacts them in var_dump/print_r/var_export, JSON
 * encoding, serialize() and toArray(); only the explicit properties read them.
 * `$webhookAuthToken` and `$webhookSigningSecret` are null from a platform
 * that predates sending them.
 */
final class ProvisionedServiceAccount implements JsonSerializable
{
    public const REDACTED = '***';

    public function __construct(
        public readonly string $message,
        public readonly string $principalId,
        public readonly string $name,
        /** The OAuth client's row id (`oac_…`). */
        public readonly string $oauthClientId,
        /** The public `client_id` of the client-credentials grant. */
        public readonly string $clientId,
        public readonly string $clientSecret,
        /** Bearer token the platform sends on its deliveries to the application. */
        public readonly ?string $webhookAuthToken = null,
        /** HMAC-SHA256 secret the platform signs its deliveries with (FLOWCATALYST_SIGNING_SECRET). */
        public readonly ?string $webhookSigningSecret = null,
    ) {}

    /** @param array<string, mixed> $data the platform's response body */
    public static function fromArray(array $data): self
    {
        $sa = is_array($data['serviceAccount'] ?? null) ? $data['serviceAccount'] : [];
        $oauth = is_array($sa['oauthClient'] ?? null) ? $sa['oauthClient'] : [];
        $webhook = is_array($sa['webhook'] ?? null) ? $sa['webhook'] : null;

        return new self(
            message: (string) ($data['message'] ?? ''),
            principalId: (string) ($sa['principalId'] ?? ''),
            name: (string) ($sa['name'] ?? ''),
            oauthClientId: (string) ($oauth['id'] ?? ''),
            clientId: (string) ($oauth['clientId'] ?? ''),
            clientSecret: (string) ($oauth['clientSecret'] ?? ''),
            webhookAuthToken: isset($webhook['authToken']) ? (string) $webhook['authToken'] : null,
            webhookSigningSecret: isset($webhook['signingSecret']) ? (string) $webhook['signingSecret'] : null,
        );
    }

    /** @return array<string, mixed> the platform's shape, every secret redacted */
    public function toArray(): array
    {
        $out = [
            'message' => $this->message,
            'serviceAccount' => [
                'principalId' => $this->principalId,
                'name' => $this->name,
                'oauthClient' => [
                    'id' => $this->oauthClientId,
                    'clientId' => $this->clientId,
                    'clientSecret' => self::REDACTED,
                ],
            ],
        ];
        if ($this->webhookAuthToken !== null || $this->webhookSigningSecret !== null) {
            $out['serviceAccount']['webhook'] = [
                'authToken' => self::REDACTED,
                'signingSecret' => self::REDACTED,
            ];
        }

        return $out;
    }

    /** @return array<string, mixed> */
    public function jsonSerialize(): array
    {
        return $this->toArray();
    }

    /** @return array<string, mixed> */
    public function __debugInfo(): array
    {
        return $this->toArray();
    }

    /** @return array<string, mixed> */
    public function __serialize(): array
    {
        return $this->toArray();
    }
}
