<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs\Ingestion;

/**
 * How a payload becomes records: `none`, `csv {header}`, `json {arrayPath}` or
 * `xml {element}`.
 */
final class IngestionSplit
{
    /**
     * @param array<string, mixed> $settings
     */
    private function __construct(
        public readonly string $kind,
        public readonly array $settings = [],
    ) {}

    /** The whole payload is one record. */
    public static function none(): self
    {
        return new self('none');
    }

    /** One record per line; `$header` drops the first line. */
    public static function csv(bool $header = true): self
    {
        return new self('csv', ['header' => $header]);
    }

    /** One record per element of the array at `$arrayPath` (`$` or `$.field.field`). */
    public static function json(string $arrayPath = '$'): self
    {
        return new self('json', ['arrayPath' => $arrayPath]);
    }

    /** One record per occurrence of `$element`. */
    public static function xml(string $element): self
    {
        return new self('xml', ['element' => $element]);
    }

    /**
     * @param array<string, mixed> $data
     */
    public static function fromArray(array $data): self
    {
        $kind = (string) ($data['kind'] ?? 'none');
        unset($data['kind']);

        return new self($kind, $data);
    }

    /**
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        return ['kind' => $this->kind] + $this->settings;
    }
}
