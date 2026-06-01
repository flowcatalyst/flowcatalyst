<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Http\Middleware;

use Closure;
use FlowCatalyst\Auth\DTOs\FlowCatalystUser;
use FlowCatalyst\Auth\Rbac\RbacCatalogue;
use FlowCatalyst\Auth\Support\AccessTokenValidator;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

/**
 * Resolves the current request's FlowCatalyst principal.
 *
 *   1. `Authorization: Bearer <token>` → validates against JWKS (RS256).
 *   2. Otherwise reads the existing session-stored principal (set by the
 *      OIDC callback flow).
 *   3. Applies the RBAC catalogue (if registered) to populate
 *      `principal->permissions`.
 *   4. Stashes the principal on the request via
 *      `$request->attributes->set('fc.principal', $user)`.
 *
 * The guard middleware ({@see RequireSession}, {@see RequireBearer},
 * {@see RequireAuth}) read that attribute to decide whether to admit the
 * request, redirect, or 401.
 *
 * Bearer wins over session on the same request: an explicit Authorization
 * header is never silently downgraded to whatever session cookie the browser
 * sent.
 */
final class AuthenticateFc
{
    public function __construct(
        private readonly AccessTokenValidator $validator,
        private readonly ?RbacCatalogue $rbac = null,
    ) {}

    public function handle(Request $request, Closure $next): Response
    {
        $principal = $this->resolve($request);
        if ($principal !== null) {
            if ($this->rbac !== null) {
                $principal = $principal->withRbac($this->rbac);
            }
            $request->attributes->set('fc.principal', $principal);
        }
        return $next($request);
    }

    private function resolve(Request $request): ?FlowCatalystUser
    {
        $bearer = $this->readBearer($request);
        if ($bearer !== null) {
            return $this->validator->validate($bearer);
        }

        // Session-stored principal (set by OidcAuthController on callback).
        if (!$request->hasSession()) {
            return null;
        }
        $stored = $request->session()->get('flowcatalyst_user');
        if (!$stored instanceof FlowCatalystUser) {
            return null;
        }
        return $stored->withMechanism('session');
    }

    private function readBearer(Request $request): ?string
    {
        $raw = $request->headers->get('Authorization');
        if (!is_string($raw)) {
            return null;
        }
        if (preg_match('/^Bearer\s+(.+)$/i', trim($raw), $m) === 1) {
            return trim($m[1]);
        }
        return null;
    }
}
