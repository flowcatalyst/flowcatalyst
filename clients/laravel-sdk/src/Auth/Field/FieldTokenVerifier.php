<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Field;

use FlowCatalyst\Auth\Support\JwksCache;
use FlowCatalyst\Auth\Support\JwtSignature;

/**
 * Verifies field-worker access tokens and their DPoP proofs (RFC 9449 style).
 * Mirrors the Go SDK's `auth.FieldTokenVerifier` and passes the same shared
 * fixtures (`api/field-fixtures`).
 *
 * Token: `Authorization: DPoP <jwt>` — JOSE `typ` `fc-field+jwt`, alg RS256
 * ONLY (verified against the platform JWKS), `iss`, `exp`, `token_use` =
 * `field`, `aud` = this application's code, no `pcr`, a `cnf.jkt`.
 * Proof: `DPoP: <jws>` — `typ` `dpop+jwt`, alg ES256 ONLY, an embedded public
 * P-256 `jwk` whose RFC 7638 thumbprint must equal `cnf.jkt`, claims `htm`,
 * `htu`, `ath` (SHA-256 of the token), `iat` within the proof window, `jti`.
 *
 * Nonce / jti replay checking is NOT done here (the proof's iat window is the
 * only freshness check); an app needing it layers its own.
 */
final class FieldTokenVerifier
{
    /** @var \Closure(): int */
    private readonly \Closure $now;

    /**
     * @param string $baseUrl FlowCatalyst platform base URL (OIDC discovery / JWKS source).
     * @param string $appCode This application's code; tokens must be addressed to it (`aud`).
     * @param int $clockSkewSeconds Bound on exp/nbf checks.
     * @param int $proofMaxAgeSeconds How far a proof's iat may be from now, either way (default 5 minutes).
     * @param (\Closure(): int)|null $now Clock override (tests, fixtures); unix seconds.
     */
    public function __construct(
        private readonly JwksCache $jwks,
        private readonly string $baseUrl,
        private readonly string $appCode,
        private readonly int $clockSkewSeconds = 60,
        private readonly int $proofMaxAgeSeconds = 300,
        ?\Closure $now = null,
    ) {
        if ($appCode === '') {
            throw new \InvalidArgumentException('FieldTokenVerifier needs the application code (flowcatalyst.field.app_code).');
        }
        $this->now = $now ?? static fn (): int => time();
    }

    /**
     * Check a request's credentials.
     *
     * @param string $authorizationHeader The Authorization header ("DPoP <token>").
     * @param string $dpopHeader The DPoP proof header.
     * @param string $method The HTTP method.
     * @param string $requestUrl The full URL the device signed: the app's
     *     configured base URL + the request path. Never built from the Host
     *     header. Any query is ignored.
     *
     * @throws FieldTokenException when the credential is refused.
     * @throws \Throwable discovery / key-fetch failures propagate as-is.
     */
    public function verify(string $authorizationHeader, string $dpopHeader, string $method, string $requestUrl): FieldPrincipal
    {
        $parts = preg_split('/\s+/', trim($authorizationHeader), 2);
        $scheme = $parts[0] ?? '';
        $token = trim($parts[1] ?? '');
        if (strcasecmp($scheme, 'DPoP') !== 0 || $token === '') {
            throw new FieldTokenException(FieldTokenException::REASON_MALFORMED, 'Authorization must be "DPoP <token>"');
        }

        $segments = explode('.', $token);
        $header = count($segments) === 3 ? JwtSignature::b64uJson($segments[0]) : null;
        if ($header === null || JwtSignature::b64uJson($segments[1]) === null) {
            throw new FieldTokenException(FieldTokenException::REASON_MALFORMED, 'not a compact JWS');
        }
        $typ = is_string($header['typ'] ?? null) ? $header['typ'] : '';
        if ($typ !== 'fc-field+jwt') {
            throw new FieldTokenException(FieldTokenException::REASON_WRONG_TYP, "typ is {$typ}");
        }
        // Field access tokens are RS256 only (JwtSignature enforces it again).
        // Refuse any other alg (HS256, none, ES256 ...) before touching keys.
        $alg = is_string($header['alg'] ?? null) ? $header['alg'] : '';
        if ($alg !== 'RS256') {
            throw new FieldTokenException(FieldTokenException::REASON_INVALID_TOKEN, "alg must be RS256, got {$alg}");
        }

        $verified = JwtSignature::verify($token, $this->jwks, $this->baseUrl);
        if ($verified === null) {
            throw new FieldTokenException(FieldTokenException::REASON_INVALID_TOKEN, 'signature does not verify');
        }
        $claims = $verified['payload'];

        $issuer = $this->jwks->issuer($this->baseUrl);
        if ($issuer === null || ($claims['iss'] ?? null) !== $issuer) {
            throw new FieldTokenException(FieldTokenException::REASON_INVALID_TOKEN, 'issuer mismatch');
        }
        $now = ($this->now)();
        if (!isset($claims['exp']) || !is_numeric($claims['exp'])) {
            throw new FieldTokenException(FieldTokenException::REASON_INVALID_TOKEN, 'exp is required');
        }
        if ((int) $claims['exp'] + $this->clockSkewSeconds < $now) {
            throw new FieldTokenException(FieldTokenException::REASON_EXPIRED);
        }
        if (isset($claims['nbf']) && is_numeric($claims['nbf']) && (int) $claims['nbf'] - $this->clockSkewSeconds > $now) {
            throw new FieldTokenException(FieldTokenException::REASON_INVALID_TOKEN, 'token not yet valid');
        }

        $tokenUse = is_string($claims['token_use'] ?? null) ? $claims['token_use'] : '';
        if ($tokenUse !== 'field') {
            throw new FieldTokenException(FieldTokenException::REASON_WRONG_TOKEN_USE, "token_use is {$tokenUse}");
        }
        $aud = $claims['aud'] ?? null;
        if (is_array($aud) && count($aud) === 1) {
            $aud = reset($aud);
        }
        if (!is_string($aud) || $aud !== $this->appCode) {
            throw new FieldTokenException(FieldTokenException::REASON_WRONG_AUD, 'aud is ' . (is_string($aud) ? $aud : 'not this application'));
        }
        if (($claims['pcr'] ?? false) === true) {
            throw new FieldTokenException(FieldTokenException::REASON_PCR, 'the token may only change the PIN');
        }
        $cnf = $claims['cnf'] ?? null;
        $cnfJkt = is_array($cnf) && is_string($cnf['jkt'] ?? null) ? $cnf['jkt'] : '';
        if ($cnfJkt === '') {
            throw new FieldTokenException(FieldTokenException::REASON_JKT_MISMATCH, 'the token carries no cnf.jkt');
        }

        if (trim($dpopHeader) === '') {
            throw new FieldTokenException(FieldTokenException::REASON_MISSING_PROOF);
        }
        $proof = $this->parseProof($dpopHeader);
        if (!hash_equals($cnfJkt, $proof['jkt'])) {
            throw new FieldTokenException(FieldTokenException::REASON_JKT_MISMATCH);
        }
        if ($proof['htm'] !== $method) {
            throw new FieldTokenException(FieldTokenException::REASON_HTM_MISMATCH);
        }
        if (!self::sameUrl($proof['htu'], $requestUrl)) {
            throw new FieldTokenException(FieldTokenException::REASON_HTU_MISMATCH);
        }
        $ath = self::b64u(hash('sha256', $token, true));
        if (!hash_equals($ath, $proof['ath'])) {
            throw new FieldTokenException(FieldTokenException::REASON_ATH_MISMATCH);
        }
        if (abs($now - $proof['iat']) > $this->proofMaxAgeSeconds) {
            throw new FieldTokenException(FieldTokenException::REASON_STALE_PROOF);
        }

        $str = static fn (string $k): string => is_string($claims[$k] ?? null) ? $claims[$k] : '';
        $amr = [];
        foreach (is_array($claims['amr'] ?? null) ? $claims['amr'] : [] as $a) {
            if (is_string($a)) {
                $amr[] = $a;
            }
        }
        $act = $claims['act'] ?? null;
        $actor = is_array($act) && is_string($act['sub'] ?? null) && $act['sub'] !== ''
            ? new FieldActor(
                id: $act['sub'],
                type: is_string($act['typ'] ?? null) ? $act['typ'] : '',
                name: is_string($act['name'] ?? null) ? $act['name'] : '',
            )
            : null;
        return new FieldPrincipal(
            workerId: $str('sub'),
            clientId: $str('client_id'),
            clientIdentifier: $str('client_identifier'),
            workerCode: $str('worker_code'),
            name: $str('name'),
            appCode: $this->appCode,
            deviceId: $str('device_id'),
            deviceMode: $str('device_mode'),
            sessionId: $str('sid'),
            amr: $amr,
            authTime: isset($claims['auth_time']) && is_numeric($claims['auth_time'])
                ? (new \DateTimeImmutable('@' . (int) $claims['auth_time']))
                : null,
            expiresAt: new \DateTimeImmutable('@' . (int) $claims['exp']),
            jkt: $proof['jkt'],
            tokenId: $str('jti'),
            actor: $actor,
            test: ($claims['test'] ?? false) === true,
        );
    }

    /**
     * Scheme, host(:port) case-insensitive and path exact; query and fragment
     * ignored on both sides.
     */
    private static function sameUrl(string $a, string $b): bool
    {
        $ua = parse_url($a);
        $ub = parse_url($b);
        if ($ua === false || $ub === false) {
            return false;
        }
        $norm = static fn (array $u): array => [
            strtolower((string) ($u['scheme'] ?? '')),
            strtolower((string) ($u['host'] ?? '')) . (isset($u['port']) ? ':' . $u['port'] : ''),
            (string) ($u['path'] ?? ''),
        ];
        return $norm($ua) === $norm($ub);
    }

    /**
     * @return array{jkt: string, htm: string, htu: string, ath: string, iat: int}
     */
    private function parseProof(string $compact): array
    {
        $bad = static fn (string $detail): FieldTokenException => new FieldTokenException(FieldTokenException::REASON_INVALID_PROOF, $detail);

        $parts = explode('.', trim($compact));
        if (count($parts) !== 3) {
            throw $bad('not a compact JWS');
        }
        $hdr = JwtSignature::b64uJson($parts[0]);
        if ($hdr === null) {
            throw $bad('header is not base64url JSON');
        }
        if (($hdr['typ'] ?? null) !== 'dpop+jwt' || ($hdr['alg'] ?? null) !== 'ES256') {
            throw $bad('typ must be dpop+jwt and alg ES256');
        }
        $jwk = $hdr['jwk'] ?? null;
        if (!is_array($jwk) || ($jwk['kty'] ?? null) !== 'EC' || ($jwk['crv'] ?? null) !== 'P-256' || isset($jwk['d'])) {
            throw $bad('jwk must be a public EC P-256 key');
        }
        $x = is_string($jwk['x'] ?? null) ? JwtSignature::b64uDecode($jwk['x']) : '';
        $y = is_string($jwk['y'] ?? null) ? JwtSignature::b64uDecode($jwk['y']) : '';
        if (strlen($x) !== 32 || strlen($y) !== 32) {
            throw $bad('jwk coordinates are malformed');
        }
        // SubjectPublicKeyInfo for id-ecPublicKey / prime256v1 + uncompressed point.
        $spki = hex2bin('3059301306072a8648ce3d020106082a8648ce3d030107034200') . "\x04" . $x . $y;
        $pem = "-----BEGIN PUBLIC KEY-----\n" . chunk_split(base64_encode($spki), 64, "\n") . "-----END PUBLIC KEY-----\n";
        $pub = @openssl_pkey_get_public($pem);
        if ($pub === false) {
            throw $bad('jwk point is not on the curve');
        }

        $sig = JwtSignature::b64uDecode($parts[2]);
        if (strlen($sig) !== 64) {
            throw $bad('signature is malformed');
        }
        // JWS ES256 is raw r||s; OpenSSL wants DER.
        $der = self::derSignature(substr($sig, 0, 32), substr($sig, 32));
        if (openssl_verify($parts[0] . '.' . $parts[1], $der, $pub, OPENSSL_ALGO_SHA256) !== 1) {
            throw $bad('signature does not verify');
        }

        $c = JwtSignature::b64uJson($parts[1]);
        if ($c === null || !isset($c['iat']) || !is_int($c['iat']) || !is_string($c['jti'] ?? null) || $c['jti'] === '') {
            throw $bad('claims need iat and jti');
        }

        $canonical = '{"crv":"P-256","kty":"EC","x":"' . self::b64u($x) . '","y":"' . self::b64u($y) . '"}';
        return [
            'jkt' => self::b64u(hash('sha256', $canonical, true)),
            'htm' => is_string($c['htm'] ?? null) ? $c['htm'] : '',
            'htu' => is_string($c['htu'] ?? null) ? $c['htu'] : '',
            'ath' => is_string($c['ath'] ?? null) ? $c['ath'] : '',
            'iat' => $c['iat'],
        ];
    }

    private static function derSignature(string $r, string $s): string
    {
        $int = static function (string $v): string {
            $v = ltrim($v, "\x00");
            if ($v === '' || (ord($v[0]) & 0x80) !== 0) {
                $v = "\x00" . $v;
            }
            return "\x02" . chr(strlen($v)) . $v;
        };
        $body = $int($r) . $int($s);
        return "\x30" . chr(strlen($body)) . $body;
    }

    private static function b64u(string $raw): string
    {
        return rtrim(strtr(base64_encode($raw), '+/', '-_'), '=');
    }
}
