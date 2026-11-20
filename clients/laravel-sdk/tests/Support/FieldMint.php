<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Support;

use FlowCatalyst\Auth\Field\FieldTokenVerifier;
use FlowCatalyst\Auth\Support\JwksCache;
use GuzzleHttp\Client;
use GuzzleHttp\Handler\MockHandler;
use GuzzleHttp\HandlerStack;
use GuzzleHttp\Psr7\Response;
use Illuminate\Cache\ArrayStore;
use Illuminate\Cache\Repository;

/**
 * Mints real field access tokens (RS256) and DPoP proofs (ES256) with keys
 * made on the spot, and builds a {@see FieldTokenVerifier} that trusts them.
 * Nothing is mocked but the platform's discovery and JWKS answers.
 */
final class FieldMint
{
    public const ISSUER = 'https://fc.example.test';
    public const APP_CODE = 'my-app';

    private \OpenSSLAsymmetricKey $rsa;
    private \OpenSSLAsymmetricKey $ec;
    private string $x;
    private string $y;
    /** @var array<string, mixed> */
    private array $jwks;

    public function __construct()
    {
        $rsa = openssl_pkey_new(['private_key_bits' => 2048, 'private_key_type' => OPENSSL_KEYTYPE_RSA]);
        $ec = openssl_pkey_new(['curve_name' => 'prime256v1', 'private_key_type' => OPENSSL_KEYTYPE_EC]);
        if ($rsa === false || $ec === false) {
            throw new \RuntimeException('could not make keys: ' . (string) openssl_error_string());
        }
        $this->rsa = $rsa;
        $this->ec = $ec;
        $d = openssl_pkey_get_details($rsa);
        $this->jwks = ['keys' => [[
            'kty' => 'RSA', 'kid' => 'mint-key', 'alg' => 'RS256', 'use' => 'sig',
            'n' => self::b64u($d['rsa']['n']), 'e' => self::b64u($d['rsa']['e']),
        ]]];
        $e = openssl_pkey_get_details($ec)['ec'];
        $this->x = str_pad($e['x'], 32, "\x00", STR_PAD_LEFT);
        $this->y = str_pad($e['y'], 32, "\x00", STR_PAD_LEFT);
    }

    public function verifier(): FieldTokenVerifier
    {
        $mock = new MockHandler([
            new Response(200, [], (string) json_encode(['issuer' => self::ISSUER, 'jwks_uri' => self::ISSUER . '/jwks'])),
            new Response(200, [], (string) json_encode($this->jwks)),
        ]);
        $cache = new JwksCache(new Client(['handler' => HandlerStack::create($mock)]), new Repository(new ArrayStore()));
        return new FieldTokenVerifier($cache, self::ISSUER, self::APP_CODE);
    }

    /** @param array<string, mixed> $claims overrides */
    public function token(array $claims = []): string
    {
        $claims = array_merge([
            'iss' => self::ISSUER,
            'aud' => self::APP_CODE,
            'sub' => 'fwu_worker1',
            'exp' => time() + 600,
            'token_use' => 'field',
            'client_id' => 'clt_acme',
            'client_identifier' => 'acme',
            'worker_code' => 'W001',
            'name' => 'Ann',
            'device_id' => 'dev_1',
            'device_mode' => 'PERSONAL',
            'sid' => 'sid_1',
            'amr' => ['pin'],
            'jti' => bin2hex(random_bytes(8)),
            'cnf' => ['jkt' => $this->jkt()],
        ], $claims);
        $signing = self::b64u((string) json_encode(['alg' => 'RS256', 'typ' => 'fc-field+jwt', 'kid' => 'mint-key']))
            . '.' . self::b64u((string) json_encode($claims));
        openssl_sign($signing, $sig, $this->rsa, OPENSSL_ALGO_SHA256);
        return $signing . '.' . self::b64u($sig);
    }

    public function proof(string $token, string $method, string $htu): string
    {
        $jwk = ['crv' => 'P-256', 'kty' => 'EC', 'x' => self::b64u($this->x), 'y' => self::b64u($this->y)];
        $signing = self::b64u((string) json_encode(['typ' => 'dpop+jwt', 'alg' => 'ES256', 'jwk' => $jwk]))
            . '.' . self::b64u((string) json_encode([
                'htm' => $method,
                'htu' => $htu,
                'ath' => self::b64u(hash('sha256', $token, true)),
                'iat' => time(),
                'jti' => bin2hex(random_bytes(8)),
            ]));
        openssl_sign($signing, $der, $this->ec, OPENSSL_ALGO_SHA256);
        return $signing . '.' . self::b64u(self::rawSignature($der));
    }

    public function jkt(): string
    {
        return self::b64u(hash('sha256', '{"crv":"P-256","kty":"EC","x":"' . self::b64u($this->x) . '","y":"' . self::b64u($this->y) . '"}', true));
    }

    /** The two headers a device sends. @return array{HTTP_AUTHORIZATION: string, HTTP_DPOP: string} */
    public function headers(string $method, string $htu, array $claims = []): array
    {
        $token = $this->token($claims);
        return ['HTTP_AUTHORIZATION' => 'DPoP ' . $token, 'HTTP_DPOP' => $this->proof($token, $method, $htu)];
    }

    /** DER ECDSA-Sig-Value to the raw r||s JWS wants. */
    private static function rawSignature(string $der): string
    {
        $off = 2;
        if ((ord($der[1]) & 0x80) !== 0) {
            $off += ord($der[1]) & 0x7f;
        }
        $out = '';
        for ($i = 0; $i < 2; $i++) {
            $len = ord($der[$off + 1]);
            $int = substr($der, $off + 2, $len);
            $out .= str_pad(ltrim($int, "\x00"), 32, "\x00", STR_PAD_LEFT);
            $off += 2 + $len;
        }
        return $out;
    }

    private static function b64u(string $raw): string
    {
        return rtrim(strtr(base64_encode($raw), '+/', '-_'), '=');
    }
}
