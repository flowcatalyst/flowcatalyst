<?php

declare(strict_types=1);

namespace FlowCatalyst\Push;

/**
 * Mints push tickets for one application with its current key, and exports the
 * JWKS (the current key then the previous ones during a rotation) that
 * `flowcatalyst:sync` sends to the gateway.
 *
 * A ticket is a short-lived grant to listen on topics, minted by the
 * application after its own access check (push contract section 6.4; the
 * gateway spec section 9): a JWS compact token, `alg` `EdDSA` (Ed25519), header
 * `kid`, claims `iss` (the application code), `aud` `fw-push`, `client`,
 * `topics`, optional `sub` and `cnf`, `iat`, `exp` (at most 5 minutes after
 * `iat`) and optional `stream-exp`.
 *
 *     $minter = TicketMinter::fromConfig('shop', config('flowcatalyst.push.ticket_key'));
 *     $ticket = $minter->mint(client: 'acme', topics: ['shop:orders:{id}'], sub: 'prn_1');
 */
final class TicketMinter
{
    public const AUDIENCE = 'fw-push';
    public const MAX_TTL_SECONDS = 300;
    public const DEFAULT_TTL_SECONDS = 60;
    public const MAX_TOPICS = 100;

    /**
     * @param array<TicketKey|array<string, string>> $previous The keys of a rotation in progress: published in the
     *        JWKS so tickets signed with them keep verifying, never used to sign. A public JWK array is enough.
     * @throws PushTicketException
     */
    public function __construct(
        public readonly string $applicationCode,
        private readonly TicketKey $current,
        private readonly array $previous = [],
    ) {
        if (preg_match('/^[a-z][a-z0-9-]*$/D', $applicationCode) !== 1) {
            throw new PushTicketException("not an application code: \"{$applicationCode}\"");
        }
        if (count($previous) > 3) {
            throw new PushTicketException('at most 4 ticket keys (the current and 3 previous)');
        }
    }

    /**
     * From configuration: the current key as `kid:base64url-seed`, and the
     * previous ones (during a rotation) as `kid:base64url-seed` strings.
     *
     * @param string[] $previous
     * @throws PushTicketException
     */
    public static function fromConfig(string $applicationCode, string $current, array $previous = []): self
    {
        $previousKeys = array_map(
            static fn(string $p) => TicketKey::parse($p),
            array_values(array_filter($previous, static fn($p) => is_string($p) && trim($p) !== '')),
        );
        return new self($applicationCode, TicketKey::parse($current), $previousKeys);
    }

    /**
     * The JWKS to sync: the current key, then the previous ones.
     *
     * @return array{keys: list<array<string, string>>}
     * @throws PushTicketException
     */
    public function jwks(): array
    {
        $keys = [$this->current->toPublicJwk()];
        foreach ($this->previous as $p) {
            $keys[] = $p instanceof TicketKey ? $p->toPublicJwk() : $p;
        }
        $kids = array_map(static fn(array $k) => $k['kid'] ?? '', $keys);
        if (count(array_unique($kids)) !== count($kids)) {
            throw new PushTicketException('duplicate ticket key ids');
        }
        return ['keys' => $keys];
    }

    /**
     * Mint a ticket. Claims, in order: iss, aud, client, topics, sub, cnf, iat, exp, stream-exp.
     *
     * @param string $client The stream's client (`_platform` for platform-wide).
     * @param string[] $topics Topic patterns of the application (placeholders `{x}` match one segment), at most 100.
     * @param string|null $sub The principal, recorded in the stream token for logs.
     * @param string|null $jkt Bind the ticket to a DPoP key: the subscribe must prove it (`cnf.jkt`).
     * @param int $ttlSeconds Lifetime, 1 to 300: the time to subscribe with it.
     * @param \DateTimeInterface|int|null $streamExp The latest the stream may live (default: exp + the gateway's max lifetime).
     * @param \DateTimeInterface|int|null $now Issued-at (default now). For tests.
     * @throws PushTicketException when the request cannot be a valid ticket
     */
    public function mint(
        string $client,
        array $topics,
        ?string $sub = null,
        ?string $jkt = null,
        int $ttlSeconds = self::DEFAULT_TTL_SECONDS,
        \DateTimeInterface|int|null $streamExp = null,
        \DateTimeInterface|int|null $now = null,
    ): string {
        if (preg_match('/^[A-Za-z0-9._-]{1,100}$/D', $client) !== 1) {
            throw new PushTicketException('client is [A-Za-z0-9._-]{1,100}');
        }
        if ($topics === [] || count($topics) > self::MAX_TOPICS) {
            throw new PushTicketException('a ticket names 1 to ' . self::MAX_TOPICS . ' topics');
        }
        foreach ($topics as $topic) {
            if (!is_string($topic) || !str_contains($topic, ':') || explode(':', $topic)[0] !== $this->applicationCode) {
                throw new PushTicketException(sprintf(
                    'the topic "%s" is not a topic of the application "%s"',
                    is_string($topic) ? $topic : gettype($topic),
                    $this->applicationCode,
                ));
            }
        }
        if ($ttlSeconds < 1 || $ttlSeconds > self::MAX_TTL_SECONDS) {
            throw new PushTicketException('ttlSeconds is 1 to ' . self::MAX_TTL_SECONDS);
        }

        $iat = self::epochSeconds($now) ?? time();
        $claims = [
            'iss' => $this->applicationCode,
            'aud' => self::AUDIENCE,
            'client' => $client,
            'topics' => array_values($topics),
        ];
        if ($sub !== null && $sub !== '') {
            $claims['sub'] = $sub;
        }
        if ($jkt !== null && $jkt !== '') {
            $claims['cnf'] = ['jkt' => $jkt];
        }
        $claims['iat'] = $iat;
        $claims['exp'] = $iat + $ttlSeconds;
        $streamExpSeconds = self::epochSeconds($streamExp);
        if ($streamExpSeconds !== null) {
            $claims['stream-exp'] = $streamExpSeconds;
        }

        $flags = JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR;
        $header = TicketKey::base64UrlEncode(json_encode(['alg' => 'EdDSA', 'kid' => $this->current->kid], $flags));
        $body = TicketKey::base64UrlEncode(json_encode($claims, $flags));
        $signingInput = "{$header}.{$body}";

        return $signingInput . '.' . TicketKey::base64UrlEncode($this->current->sign($signingInput));
    }

    private static function epochSeconds(\DateTimeInterface|int|null $value): ?int
    {
        if ($value === null) {
            return null;
        }
        return $value instanceof \DateTimeInterface ? $value->getTimestamp() : $value;
    }
}
