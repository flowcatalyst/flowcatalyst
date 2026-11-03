<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Support;

use FlowCatalyst\Auth\DTOs\FlowCatalystUser;

/**
 * Validates FlowCatalyst access tokens (Bearer) using JWKS — RS256 only.
 *
 * Verification path (signature + kid lookup delegated to {@see JwtSignature}):
 *   1. Parse JWT, lift `kid` from the header, verify the RS256 signature
 *      against the matching JWK from {@see JwksCache::keys()}.
 *   2. Check `exp`, `nbf`, `sub`, and `token_use` (allowlist: `api`, `identity` or absent).
 *   3. Check `iss` against the platform's own discovery issuer
 *      ({@see JwksCache::issuer()}) — mandatory, not best-effort.
 *   4. Check `aud` — only when `$expectedAudience` is configured (opt-in).
 *
 * Implemented without a JWT library to keep the dependency surface small and
 * avoid security-advisory churn in the JWT-library ecosystem. The same token
 * shape applies to both `authorization_code` and `client_credentials` grants.
 */
final class AccessTokenValidator
{
    public function __construct(
        private readonly JwksCache $jwks,
        private readonly string $baseUrl,
        private readonly ?string $expectedAudience = null,
    ) {}

    /**
     * Validate a Bearer access token and return a Principal. Returns null
     * on any validation failure (caller decides how to respond).
     */
    public function validate(string $token): ?FlowCatalystUser
    {
        $verified = JwtSignature::verify($token, $this->jwks, $this->baseUrl);
        if ($verified === null) {
            return null;
        }
        $payload = $verified['payload'];

        // Standard temporal claim checks.
        $now = time();
        if (isset($payload['exp']) && is_numeric($payload['exp']) && (int) $payload['exp'] < $now) {
            return null;
        }
        if (isset($payload['nbf']) && is_numeric($payload['nbf']) && (int) $payload['nbf'] > $now + 60) {
            return null;
        }
        if (!isset($payload['sub']) || !is_string($payload['sub'])) {
            return null;
        }

        // `token_use` is an allowlist, not a denylist: an API credential
        // (`api`), an interactive login's access token (`identity`, accepted
        // here as before so existing apps keep working) or a token with no
        // marker. Any other class (`field`, a future one) shares the platform
        // signing keys but is never accepted by this guard.
        $tokenUse = $payload['token_use'] ?? null;
        if ($tokenUse !== null && $tokenUse !== 'api' && $tokenUse !== 'identity') {
            return null;
        }

        // `iss` must match the platform's own discovery issuer. Fail closed
        // if the discovery doc did not carry one — never treat "unknown" as
        // "acceptable".
        $issuer = $this->jwks->issuer($this->baseUrl);
        if ($issuer === null || ($payload['iss'] ?? null) !== $issuer) {
            return null;
        }

        if ($this->expectedAudience !== null) {
            $aud = $payload['aud'] ?? null;
            $audMatch = is_string($aud)
                ? $aud === $this->expectedAudience
                : (is_array($aud) && in_array($this->expectedAudience, $aud, true));
            if (!$audMatch) {
                return null;
            }
        }

        return FlowCatalystUser::fromAccessTokenClaims(
            claims: $payload,
            accessToken: $token,
            mechanism: 'bearer',
        );
    }
}
