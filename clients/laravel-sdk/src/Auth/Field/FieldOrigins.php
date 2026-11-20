<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Field;

use Symfony\Component\HttpFoundation\Exception\SuspiciousOperationException;
use Symfony\Component\HttpFoundation\Request as SymfonyRequest;

/**
 * The origins a field-worker request may have been made to
 * (`flowcatalyst.field.app_base_url`).
 *
 * The configuration is one origin, a list of origins or a comma-separated
 * string of them. An entry may have `*` as the whole leftmost label of its
 * host (`https://*.example.com`), which matches exactly one label.
 *
 * One entry with no wildcard is the fixed-origin case: the request's host is
 * never read. Otherwise {@see self::forRequest()} reads the request's host,
 * finds the first entry it matches and builds the origin from THAT ENTRY's
 * scheme and port plus the host. The request's own scheme and port are never
 * used: behind a load balancer they are not what the device called.
 *
 * The host is untrusted input, but it only chooses among origins the
 * application's own configuration allows. Someone who gets a device to call a
 * host of theirs holds a proof for that host, which matches no entry. The one
 * condition: every wildcard must cover a DNS zone the application's owner
 * wholly controls.
 *
 * A path, query or fragment on an entry is ignored (an `APP_URL` with a path
 * has always been read as its origin).
 */
final class FieldOrigins
{
    private const MISSING = 'The field guard needs flowcatalyst.field.app_base_url (or APP_URL): the public origin '
        . 'of this application. The Host header is never used.';

    /**
     * @param list<array{scheme: string, host: string, port: ?int, wildcard: bool, raw: string}> $entries
     */
    private function __construct(private readonly array $entries) {}

    /**
     * @param string|list<string>|null $config
     * @throws \LogicException when the configuration is empty or an entry is invalid.
     */
    public static function fromConfig(string|array|null $config): self
    {
        if (is_array($config)) {
            $raw = [];
            foreach ($config as $item) {
                if (is_string($item)) {
                    array_push($raw, ...explode(',', $item));
                }
            }
        } else {
            $raw = explode(',', (string) $config);
        }
        $raw = array_values(array_filter(array_map('trim', $raw), static fn (string $s): bool => $s !== ''));
        if ($raw === []) {
            throw new \LogicException(self::MISSING);
        }
        $entries = [];
        foreach ($raw as $entry) {
            $entries[] = self::parseEntry($entry, count($raw) === 1);
        }
        return new self($entries);
    }

    /** True for one entry with no wildcard: the request's host is then not read. */
    public function isFixed(): bool
    {
        return count($this->entries) === 1 && !$this->entries[0]['wildcard'];
    }

    /** The origin of the fixed-origin case. */
    public function fixedOrigin(): string
    {
        // Read exactly as a single configured origin always was.
        return (string) FieldGuard::origin($this->entries[0]['raw']);
    }

    /**
     * The origin to expect for this request, or null when its host matches no
     * entry (or is not a valid host).
     */
    public function forRequest(SymfonyRequest $request): ?string
    {
        if ($this->isFixed()) {
            return $this->fixedOrigin();
        }
        try {
            $host = $request->getHost();
        } catch (SuspiciousOperationException) {
            return null; // a malformed host
        }
        $host = strtolower($host);
        if (str_ends_with($host, '.')) {
            $host = substr($host, 0, -1);
        }
        if ($host === '') {
            return null;
        }
        foreach ($this->entries as $entry) {
            if (self::matches($entry, $host)) {
                return self::origin($entry, $host);
            }
        }
        return null;
    }

    /** @param array{scheme: string, host: string, port: ?int, wildcard: bool, raw: string} $entry */
    private static function matches(array $entry, string $host): bool
    {
        if (!$entry['wildcard']) {
            return $host === $entry['host'];
        }
        $zone = '.' . substr($entry['host'], 2); // "*.example.com" -> ".example.com"
        if (!str_ends_with($host, $zone)) {
            return false;
        }
        $label = substr($host, 0, strlen($host) - strlen($zone));
        // Exactly one non-empty label, as a TLS wildcard matches.
        return $label !== '' && !str_contains($label, '.');
    }

    /** @param array{scheme: string, host: string, port: ?int, wildcard: bool, raw: string} $entry */
    private static function origin(array $entry, string $host): string
    {
        return $entry['scheme'] . '://' . $host . ($entry['port'] !== null ? ':' . $entry['port'] : '');
    }

    /** @return array{scheme: string, host: string, port: ?int, wildcard: bool, raw: string} */
    private static function parseEntry(string $entry, bool $single): array
    {
        $p = parse_url($entry);
        if ($p === false || empty($p['scheme']) || empty($p['host'])) {
            if ($single && !str_contains($entry, '*')) {
                throw new \LogicException(self::MISSING);
            }
            throw new \LogicException("flowcatalyst.field.app_base_url entry \"{$entry}\" needs a scheme and a host.");
        }
        $host = strtolower($p['host']);
        if (str_ends_with($host, '.')) {
            $host = substr($host, 0, -1);
        }
        $wildcard = false;
        if (str_contains($host, '*')) {
            $zone = str_starts_with($host, '*.') ? substr($host, 2) : '';
            $labels = $zone === '' ? [] : explode('.', $zone);
            $ok = count($labels) >= 2 && !str_contains($zone, '*');
            foreach ($labels as $label) {
                $ok = $ok && $label !== '';
            }
            if (!$ok) {
                throw new \LogicException(
                    "flowcatalyst.field.app_base_url entry \"{$entry}\": a * must be the whole leftmost label of the host "
                    . 'and be followed by a zone of at least two labels (https://*.example.com).'
                );
            }
            $wildcard = true;
        }
        return [
            'scheme' => strtolower($p['scheme']),
            'host' => $host,
            'port' => isset($p['port']) ? (int) $p['port'] : null,
            'wildcard' => $wildcard,
            'raw' => $entry,
        ];
    }
}
