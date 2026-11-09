<?php

declare(strict_types=1);

namespace FlowCatalyst\Push;

/**
 * A topic rule (push contract section 6.3): who may listen to a topic pattern
 * without a ticket. A pattern's segments are literals or placeholders:
 * `{principal}` must equal the subscriber's principal id, `{client}` the
 * stream's client, any other `{name}` matches one segment. The first segment is
 * the application's code.
 */
final class PushRuleDefinition
{
    /**
     * @param string $pattern `{application}:{segment}(:{segment})*`
     * @param string[] $requires Permissions the subscriber must all hold
     * @param bool $public A deliberately open topic: anyone may listen, with or without credentials.
     *        Required for a rule with neither `{principal}` nor `requires`; a public rule cannot
     *        also require permissions.
     */
    public function __construct(
        public readonly string $pattern,
        public readonly array $requires = [],
        public readonly bool $public = false,
    ) {}

    /**
     * @param array{pattern: string, requires?: string[], public?: bool}|self $rule
     */
    public static function from(array|self $rule): self
    {
        if ($rule instanceof self) {
            return $rule;
        }
        return new self(
            (string) ($rule['pattern'] ?? ''),
            array_values(array_map('strval', (array) ($rule['requires'] ?? []))),
            (bool) ($rule['public'] ?? false),
        );
    }

    /**
     * @return array{pattern: string, requires?: string[], public?: bool}
     */
    public function toArray(): array
    {
        $out = ['pattern' => $this->pattern];
        if ($this->requires !== []) {
            $out['requires'] = $this->requires;
        }
        if ($this->public) {
            $out['public'] = true;
        }
        return $out;
    }
}
