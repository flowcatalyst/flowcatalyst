<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Field;

use Illuminate\Auth\GuardHelpers;
use Illuminate\Contracts\Auth\Guard;
use Illuminate\Http\Request;

/**
 * The `field` auth guard: builds a {@see FieldPrincipal} from the request's
 * `Authorization: DPoP <token>` + `DPoP: <proof>` headers. Stateless; the
 * verification result (principal or refusal) is memoised per request object.
 *
 * `htu` is the configured app base URL + the request path, NEVER the Host
 * header (a client controls Host, and behind a proxy it is rarely what the
 * device signed).
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
     */
    public function __construct(
        private readonly FieldTokenVerifier $verifier,
        private readonly \Closure $requestResolver,
        private readonly string $appBaseUrl,
    ) {}

    public function user(): ?FieldPrincipal
    {
        $request = ($this->requestResolver)();
        if ($this->checked !== $request) {
            $this->checked = $request;
            $this->refusal = null;
            $this->user = null;
            try {
                $this->user = $this->verifier->verify(
                    (string) $request->headers->get('Authorization', ''),
                    (string) $request->headers->get('DPoP', ''),
                    $request->getMethod(),
                    $this->requestUrl($request),
                );
            } catch (FieldTokenException $e) {
                $this->refusal = $e;
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

    private function requestUrl(Request $request): string
    {
        $origin = self::origin($this->appBaseUrl);
        if ($origin === null) {
            throw new \LogicException(
                'The field guard needs flowcatalyst.field.app_base_url (or APP_URL): the public origin '
                . 'of this application. The Host header is never used.'
            );
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
