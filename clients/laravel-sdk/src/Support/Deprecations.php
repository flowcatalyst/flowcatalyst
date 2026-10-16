<?php

declare(strict_types=1);

namespace FlowCatalyst\Support;

/**
 * Once-per-process deprecation notices.
 *
 * Each key fires a single `E_USER_DEPRECATED` per PHP process. The notice is
 * suppressible the usual ways (`error_reporting()`, `@`, a custom handler), and
 * the key is marked as seen before it is raised so a handler that throws still
 * only ever sees it once.
 */
final class Deprecations
{
    /** @var array<string, true> */
    private static array $seen = [];

    public static function once(string $key, string $message): void
    {
        if (isset(self::$seen[$key])) {
            return;
        }

        self::$seen[$key] = true;

        trigger_error($message, E_USER_DEPRECATED);
    }

    /**
     * Forget which notices have fired (tests only).
     *
     * @internal
     */
    public static function reset(): void
    {
        self::$seen = [];
    }
}
