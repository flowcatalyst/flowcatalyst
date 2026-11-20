<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Contracts;

use FlowCatalyst\Auth\Field\FieldPrincipal;
use Illuminate\Http\Request;

/**
 * Decides whether a verified field worker may use this request.
 *
 * The SDK checks that the token is for this application (`aud`), but not which
 * client the request is for. An application that serves several clients, each
 * on its own host, binds an implementation to keep one client's workers out of
 * another client's data, for example by comparing the worker's
 * `clientIdentifier` with the client the request's host belongs to.
 *
 * The SDK binds no implementation. Without one, every verified worker is
 * allowed, as before. Bind yours in a service provider:
 *
 *     $this->app->bind(FieldClientCheck::class, MyFieldClientCheck::class);
 *
 * The `field` guard calls it once per request, after the token and the proof
 * verified and the token is not a PIN-change-only one. `false` refuses the
 * request with the reason `wrong_client`; the `fc.field` middleware answers it
 * with 403 `{"error":"WRONG_CLIENT"}`. An exception thrown here is not turned
 * into a refusal: it surfaces as any unexpected error does (the middleware
 * reports it and answers 503).
 *
 * `FieldTokenVerifier::verify()` called directly does not run this check.
 */
interface FieldClientCheck
{
    /** Whether a verified field worker may use this request. */
    public function allows(FieldPrincipal $worker, Request $request): bool;
}
