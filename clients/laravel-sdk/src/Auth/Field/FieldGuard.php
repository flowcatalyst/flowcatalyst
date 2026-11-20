<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Field;

use FlowCatalyst\Auth\Contracts\FieldClientCheck;
use Illuminate\Auth\GuardHelpers;
use Illuminate\Contracts\Auth\Guard;
use Illuminate\Http\Request;

/**
 * The `field` auth guard: builds a {@see FieldPrincipal} from the request's
 * `Authorization: DPoP <token>` + `DPoP: <proof>` headers. Stateless; the
 * verification result (principal or refusal) is memoised per request object.
 *
 * `htu` is a configured origin + the request path. With one configured origin
 * (no wildcard) the Host header is NEVER read (a client controls Host, and
 * behind a proxy it is rarely what the device signed). With several origins or
 * a wildcard, the request's host only chooses among the configured origins;
 * the scheme and port come from the matching entry ({@see FieldOrigins}). A host
 * that matches none is refused as `htu_mismatch`, before the token is read.
 *
 * When the application binds a {@see FieldClientCheck}, it runs once per
 * request after the token and proof verified; `false` is the reason
 * `wrong_client`. With several origins or a wildcard a check is REQUIRED: the
 * application then almost certainly serves several clients, so with none bound
 * the guard throws a LogicException on the first field request, before the
 * token is read. One fixed origin needs none.
 *
 * `Auth::guard('field')->user()` is null when the request is refused; the
 * reason is in {@see self::refusal()}. Use the `fc.field` middleware for the
 * proper 401 / 403 responses.
 */
final class FieldGuard implements Guard
{
    use GuardHelpers;

    private ?Request $checked = null;
    private ?FieldTokenException $refusal = null;

    /**
     * @param \Closure(): Request $requestResolver
     * @param string|list<string>|null $appBaseUrl one origin, a list, or a comma-separated string; see {@see FieldOrigins}
     * @param (\Closure(): ?FieldClientCheck)|null $clientCheck resolves the application's check at request time (null: none bound)
     */
    public function __construct(
        private readonly FieldTokenVerifier $verifier,
        private readonly \Closure $requestResolver,
        private readonly string|array|null $appBaseUrl,
        private readonly ?\Closure $clientCheck = null,
    ) {}

    public function user(): ?FieldPrincipal
    {
        $request = ($this->requestResolver)();
        if ($this->checked !== $request) {
            $this->checked = $request;
            $this->refusal = null;
            $this->user = null;
            try {
                $origins = FieldOrigins::fromConfig($this->appBaseUrl); // an invalid entry is reported first
                $check = $this->clientCheck === null ? null : ($this->clientCheck)();
                if ($check === null && !$origins->isFixed()) {
                    throw new \LogicException(
                        'flowcatalyst.field.app_base_url has several origins or a wildcard, so this application serves '
                        . 'more than one client, and the SDK will not run without a way to tell which client a request '
                        . 'is for. Bind a FlowCatalyst\\Auth\\Contracts\\FieldClientCheck that returns true only for a worker of '
                        . 'the client the request is for. An application with one client on several hosts binds one that returns true.'
                    );
                }
                $url = $this->requestUrl($request, $origins);
                if ($url === null) {
                    throw new FieldTokenException(FieldTokenException::REASON_HTU_MISMATCH, 'the request host is not one of the configured origins');
                }
                $principal = $this->verifier->verify(
                    (string) $request->headers->get('Authorization', ''),
                    (string) $request->headers->get('DPoP', ''),
                    $request->getMethod(),
                    $url,
                );
                if ($check !== null && !$check->allows($principal, $request)) {
                    throw new FieldTokenException(FieldTokenException::REASON_WRONG_CLIENT);
                }
                $this->user = $principal;
            } catch (FieldTokenException $e) {
                $this->refusal = $e;
            } catch (\LogicException $e) {
                $this->checked = null; // a configuration error is not a refusal: every call raises it again
                throw $e;
            }
        }
        /** @var FieldPrincipal|null */
        return $this->user;
    }

    /** @param array<string, mixed> $credentials */
    public function validate(array $credentials = []): bool
    {
        return false;
    }

    /** Why the current request was refused, if it was. */
    public function refusal(): ?FieldTokenException
    {
        $this->user();
        return $this->refusal;
    }

    /** The address the device should have called, or null when the request's host is not allowed. */
    private function requestUrl(Request $request, FieldOrigins $origins): ?string
    {
        $origin = $origins->forRequest($request);
        if ($origin === null) {
            return null;
        }
        $path = parse_url($request->getRequestUri(), PHP_URL_PATH);
        return $origin . (is_string($path) ? $path : '/');
    }

    /** scheme://host[:port] of a configured URL, or null if it has none. */
    public static function origin(string $url): ?string
    {
        $p = parse_url(trim($url));
        if ($p === false || empty($p['scheme']) || empty($p['host'])) {
            return null;
        }
        return strtolower($p['scheme']) . '://' . $p['host'] . (isset($p['port']) ? ':' . $p['port'] : '');
    }
}
