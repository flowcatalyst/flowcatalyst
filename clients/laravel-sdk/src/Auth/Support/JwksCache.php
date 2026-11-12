<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Support;

use GuzzleHttp\Client as HttpClient;
use Illuminate\Contracts\Cache\Repository as Cache;

/**
 * Caches OIDC discovery + JWKS keys per issuer with a TTL.
 *
 * Lazy first-use fetch of `{baseUrl}/.well-known/openid-configuration`,
 * then `{jwks_uri}`. Stored under the Laravel cache so concurrent workers
 * share them.
 *
 * No JWT library dependency — we keep the raw JWK objects and let
 * {@see AccessTokenValidator} verify signatures via OpenSSL directly.
 */
final class JwksCache
{
    public function __construct(
        private readonly HttpClient $http,
        private readonly Cache $cache,
        private readonly int $ttlSeconds = 300,
        /** Minimum seconds between forced refetches (unknown `kid`) per issuer. */
        private readonly int $forcedRefetchIntervalSeconds = 60,
        /** Injectable clock (unix seconds) so tests need no sleeps. */
        private readonly ?\Closure $clock = null,
    ) {}

    /**
     * Return JWKS entries keyed by `kid` for the platform at `$baseUrl`.
     * Each entry is the raw JWK as an associative array (n, e, kid, alg…).
     *
     * @return array<string, array<string, mixed>>
     */
    public function keys(string $baseUrl): array
    {
        return $this->fetch($baseUrl)['keys'];
    }

    /**
     * The `issuer` claim value the platform's own discovery document
     * advertises for `$baseUrl`, or null if the discovery doc did not carry
     * one. This is what {@see AccessTokenValidator} and {@see IdTokenValidator}
     * compare a token's `iss` claim against — never a value the caller
     * supplies, so a token cannot claim its own way past the check.
     */
    public function issuer(string $baseUrl): ?string
    {
        $issuer = $this->fetch($baseUrl)['issuer'] ?? null;
        return is_string($issuer) && $issuer !== '' ? $issuer : null;
    }

    /**
     * Keys for a token whose `kid` is absent from the cached set.
     *
     * Allows at most ONE forced refetch per issuer per
     * `$forcedRefetchIntervalSeconds` (the attempt is stamped whether or not it
     * succeeds, and the stamp lives in the shared cache). Inside the window the
     * cached set is returned untouched (no network call). A failed refetch
     * keeps the previously cached set until its normal expiry.
     *
     * @return array<string, array<string, mixed>>
     */
    public function keysAfterUnknownKid(string $baseUrl): array
    {
        $stampKey = 'fc.jwks.forced.' . sha1($baseUrl);
        $now = $this->now();
        $last = $this->cache->get($stampKey);
        if (is_int($last) && $now - $last < $this->forcedRefetchIntervalSeconds) {
            return $this->keys($baseUrl);
        }
        $this->cache->put($stampKey, $now, max($this->forcedRefetchIntervalSeconds, 1));

        $previous = $this->cache->get('fc.jwks.' . sha1($baseUrl));
        try {
            return $this->load($baseUrl)['keys'];
        } catch (\Throwable) {
            // Keep the previously cached key set (do not drop it) on failure.
            if (is_array($previous) && isset($previous['keys'])) {
                return $previous['keys'];
            }
            throw new \RuntimeException('JWKS refetch failed and no cached key set exists');
        }
    }

    private function now(): int
    {
        return $this->clock !== null ? (int) ($this->clock)() : time();
    }

    public function invalidate(string $baseUrl): void
    {
        $this->cache->forget('fc.jwks.' . sha1($baseUrl));
    }

    /**
     * @return array{keys: array<string, array<string, mixed>>, issuer: string|null}
     */
    private function fetch(string $baseUrl): array
    {
        $cacheKey = 'fc.jwks.' . sha1($baseUrl);
        $cached = $this->cache->get($cacheKey);
        if (is_array($cached) && isset($cached['keys'])) {
            /** @var array{keys: array<string, array<string, mixed>>, issuer: string|null} */
            return $cached;
        }

        return $this->load($baseUrl);
    }

    /**
     * Network fetch of discovery + JWKS; writes the cache on success only.
     *
     * @return array{keys: array<string, array<string, mixed>>, issuer: string|null}
     */
    private function load(string $baseUrl): array
    {
        $cacheKey = 'fc.jwks.' . sha1($baseUrl);
        $base = rtrim($baseUrl, '/');
        $discoveryRaw = $this->http
            ->get($base . '/.well-known/openid-configuration', [
                'headers' => ['Accept' => 'application/json'],
            ])
            ->getBody()
            ->getContents();
        /** @var array<string, mixed> $doc */
        $doc = json_decode($discoveryRaw, true, 512, JSON_THROW_ON_ERROR);
        $jwksUri = $doc['jwks_uri'] ?? throw new \RuntimeException('discovery doc missing jwks_uri');
        if (!is_string($jwksUri)) {
            throw new \RuntimeException('discovery doc jwks_uri is not a string');
        }

        $jwksRaw = $this->http
            ->get($jwksUri, ['headers' => ['Accept' => 'application/json']])
            ->getBody()
            ->getContents();
        /** @var array<string, mixed> $jwks */
        $jwks = json_decode($jwksRaw, true, 512, JSON_THROW_ON_ERROR);
        if (!isset($jwks['keys']) || !is_array($jwks['keys'])) {
            throw new \RuntimeException('JWKS document missing keys array');
        }

        $byKid = [];
        foreach ($jwks['keys'] as $key) {
            if (!is_array($key) || !isset($key['kid']) || !is_string($key['kid'])) {
                continue;
            }
            $byKid[$key['kid']] = $key;
        }

        $result = ['keys' => $byKid, 'issuer' => is_string($doc['issuer'] ?? null) ? $doc['issuer'] : null];
        $this->cache->put($cacheKey, $result, $this->ttlSeconds);
        return $result;
    }
}
