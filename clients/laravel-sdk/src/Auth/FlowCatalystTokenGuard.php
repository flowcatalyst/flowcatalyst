<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth;

use FlowCatalyst\Auth\Rbac\RbacCatalogue;
use FlowCatalyst\Auth\Support\AccessTokenValidator;
use Illuminate\Http\Request;

/**
 * Resolves a {@see FlowCatalystAuthenticatable} from a request's Bearer access
 * token (works for both user and service-account / client_credentials tokens).
 *
 * Returns null for any request that does NOT carry a valid FlowCatalyst token
 * — which is exactly what lets it sit in front of Passport: a non-FlowCatalyst
 * caller falls through to the existing auth chain instead of being rejected.
 *
 * Powers the `fc-token` request guard (use `auth:fc-token` or
 * `auth:fc-token,api`) and the {@see Http\Middleware\AuthenticateServiceTokenOrFallback}
 * dispatcher.
 */
final class FlowCatalystTokenGuard
{
    public function __construct(
        private readonly AccessTokenValidator $validator,
        private readonly ?RbacCatalogue $rbac = null,
    ) {
    }

    public function resolve(Request $request): ?FlowCatalystAuthenticatable
    {
        $token = $this->bearer($request);
        if ($token === null) {
            return null;
        }
        $user = $this->validator->validate($token);
        if ($user === null) {
            return null;
        }
        if ($this->rbac !== null) {
            $user = $user->withRbac($this->rbac);
        }
        return new FlowCatalystAuthenticatable($user->withMechanism('token'));
    }

    private function bearer(Request $request): ?string
    {
        $raw = $request->headers->get('Authorization');
        if (! is_string($raw)) {
            return null;
        }
        if (preg_match('/^Bearer\s+(.+)$/i', trim($raw), $m) === 1) {
            return trim($m[1]);
        }
        return null;
    }
}
