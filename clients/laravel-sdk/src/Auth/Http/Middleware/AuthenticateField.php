<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Http\Middleware;

use Closure;
use FlowCatalyst\Auth\Field\FieldGuard;
use FlowCatalyst\Auth\Field\FieldTokenException;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Symfony\Component\HttpFoundation\Response;

/**
 * `fc.field` — authenticates field-worker requests with the `field` guard.
 *
 * - Verified: the {@see \FlowCatalyst\Auth\Field\FieldPrincipal} is on
 *   `$request->attributes->get('fc.field_principal')` and `$request->user()`.
 * - `pcr` (PIN change required) token: 403 `PIN_CHANGE_REQUIRED`.
 * - The application's `FieldClientCheck` refused the worker: 403 `WRONG_CLIENT`, with no
 *   `WWW-Authenticate` (the worker's session is good; they called the wrong client's address).
 * - Any other refusal: 401 `{"error": <reason>}` with
 *   `WWW-Authenticate: DPoP error="invalid_token"`.
 * - Key-fetch / discovery failure: 503 `{"error": "unavailable"}`.
 */
final class AuthenticateField
{
    public function handle(Request $request, Closure $next): Response
    {
        /** @var FieldGuard $guard */
        $guard = Auth::guard('field');
        try {
            $principal = $guard->user();
        } catch (\LogicException $e) {
            throw $e; // misconfiguration (no or invalid app base URL): a loud 500, not a 401
        } catch (\Throwable $e) {
            report($e);
            return response()->json(['error' => 'unavailable'], Response::HTTP_SERVICE_UNAVAILABLE);
        }
        if ($principal !== null) {
            $request->attributes->set('fc.field_principal', $principal);
            $request->setUserResolver(static fn () => $principal);
            return $next($request);
        }

        $refusal = $guard->refusal();
        if ($refusal?->reason === FieldTokenException::REASON_PCR) {
            return response()->json(['error' => 'PIN_CHANGE_REQUIRED'], Response::HTTP_FORBIDDEN);
        }
        if ($refusal?->reason === FieldTokenException::REASON_WRONG_CLIENT) {
            return response()->json(['error' => 'WRONG_CLIENT'], Response::HTTP_FORBIDDEN);
        }
        return response()->json(['error' => $refusal?->reason ?? FieldTokenException::REASON_INVALID_TOKEN], Response::HTTP_UNAUTHORIZED, [
            'WWW-Authenticate' => 'DPoP error="invalid_token"',
        ]);
    }
}
