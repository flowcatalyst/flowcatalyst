<?php

declare(strict_types=1);

namespace FlowCatalyst\DTOs;

/**
 * A worker's photo as {@see \FlowCatalyst\Client\Resources\FieldWorkers::getPhoto()} reads it.
 */
final class FieldWorkerPhotoImage
{
    /**
     * @param string|null $bytes       the JPEG; null when not modified
     * @param string|null $etag        the validator ("<version>-<size>", quotes included): pass it as `$ifNoneMatch` next time
     * @param bool        $notModified true when the platform answered 304 to `$ifNoneMatch`
     */
    public function __construct(
        public readonly ?string $bytes,
        public readonly ?string $contentType,
        public readonly ?string $etag,
        public readonly bool $notModified,
    ) {}
}
