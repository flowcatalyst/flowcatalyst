<?php

declare(strict_types=1);

namespace FlowCatalyst\Push;

/**
 * An Ed25519 ticket signing key: a `kid` plus the 32-byte seed. Configure it as
 * `kid:base64url-seed` ({@see parse()}); the seed is a secret, the derived
 * public JWK is what the gateway needs (push contract 6.4).
 *
 * Needs ext-sodium (bundled with PHP).
 */
final class TicketKey
{
    private function __construct(
        public readonly string $kid,
        private readonly string $secretKey,
        private readonly string $publicKey,
    ) {}

    /**
     * From a `kid:base64url-seed` string (the seed is the 32 raw bytes, base64url without padding).
     *
     * @throws PushTicketException
     */
    public static function parse(string $config): self
    {
        $pos = strpos($config, ':');
        if ($pos === false || $pos === 0 || $pos === strlen($config) - 1) {
            throw new PushTicketException('a ticket key is kid:base64url-seed');
        }
        return self::fromSeed(trim(substr($config, 0, $pos)), trim(substr($config, $pos + 1)));
    }

    /**
     * @throws PushTicketException
     */
    public static function fromSeed(string $kid, string $seedBase64Url): self
    {
        if (!function_exists('sodium_crypto_sign_seed_keypair')) {
            throw new PushTicketException('ticket keys need ext-sodium');
        }
        if ($kid === '' || strlen($kid) > 64) {
            throw new PushTicketException('a ticket key id is 1 to 64 characters');
        }
        $seed = self::base64UrlDecode($seedBase64Url);
        if ($seed === null || strlen($seed) !== SODIUM_CRYPTO_SIGN_SEEDBYTES) {
            throw new PushTicketException('a ticket key seed is 32 bytes, base64url');
        }
        $pair = sodium_crypto_sign_seed_keypair($seed);
        $key = new self($kid, sodium_crypto_sign_secretkey($pair), sodium_crypto_sign_publickey($pair));
        sodium_memzero($seed);
        return $key;
    }

    /**
     * The public key as a JWK (no private member): what sync sends and
     * `GET /push/keys` serves.
     *
     * @return array{kty: string, crv: string, x: string, kid: string, use: string, alg: string}
     */
    public function toPublicJwk(): array
    {
        return [
            'kty' => 'OKP',
            'crv' => 'Ed25519',
            'x' => self::base64UrlEncode($this->publicKey),
            'kid' => $this->kid,
            'use' => 'sig',
            'alg' => 'EdDSA',
        ];
    }

    /**
     * Ed25519 signature (64 bytes) of $data.
     *
     * @internal
     */
    public function sign(string $data): string
    {
        return sodium_crypto_sign_detached($data, $this->secretKey);
    }

    public static function base64UrlEncode(string $bytes): string
    {
        return rtrim(strtr(base64_encode($bytes), '+/', '-_'), '=');
    }

    public static function base64UrlDecode(string $text): ?string
    {
        if (preg_match('/^[A-Za-z0-9_-]*$/D', $text) !== 1) {
            return null;
        }
        $decoded = base64_decode(strtr($text, '-_', '+/') . str_repeat('=', (4 - strlen($text) % 4) % 4), true);
        return $decoded === false ? null : $decoded;
    }

    public function __debugInfo(): array
    {
        return ['kid' => $this->kid];
    }
}
