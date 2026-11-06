<?php

namespace FlowCatalyst\Generated\Normalizer;

use Jane\Component\JsonSchemaRuntime\Reference;
use FlowCatalyst\Generated\Runtime\Normalizer\CheckArray;
use FlowCatalyst\Generated\Runtime\Normalizer\ValidatorTrait;
use Symfony\Component\Serializer\Normalizer\DenormalizerAwareInterface;
use Symfony\Component\Serializer\Normalizer\DenormalizerAwareTrait;
use Symfony\Component\Serializer\Normalizer\DenormalizerInterface;
use Symfony\Component\Serializer\Normalizer\NormalizerAwareInterface;
use Symfony\Component\Serializer\Normalizer\NormalizerAwareTrait;
use Symfony\Component\Serializer\Normalizer\NormalizerInterface;
class IngestionTraceRecordResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionTraceRecordResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionTraceRecordResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionTraceRecordResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('redacted', $data) && \is_int($data['redacted'])) {
            $data['redacted'] = (bool) $data['redacted'];
        }
        if (\array_key_exists('at', $data) && $data['at'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['at']);
            if (false === $date) {
                if (is_string($data['at']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['at'])) {
                    try {
                        $date = new \DateTime($data['at']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['at'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['at'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setAt($date);
        }
        elseif (\array_key_exists('at', $data) && $data['at'] === null) {
            $object->setAt(null);
        }
        if (\array_key_exists('captureId', $data) && $data['captureId'] !== null) {
            $object->setCaptureId($data['captureId']);
        }
        elseif (\array_key_exists('captureId', $data) && $data['captureId'] === null) {
            $object->setCaptureId(null);
        }
        if (\array_key_exists('data', $data) && $data['data'] !== null) {
            $object->setData($data['data']);
        }
        elseif (\array_key_exists('data', $data) && $data['data'] === null) {
            $object->setData(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
        }
        if (\array_key_exists('level', $data) && $data['level'] !== null) {
            $object->setLevel($data['level']);
        }
        elseif (\array_key_exists('level', $data) && $data['level'] === null) {
            $object->setLevel(null);
        }
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
        }
        if (\array_key_exists('redacted', $data) && $data['redacted'] !== null) {
            $object->setRedacted($data['redacted']);
        }
        elseif (\array_key_exists('redacted', $data) && $data['redacted'] === null) {
            $object->setRedacted(null);
        }
        if (\array_key_exists('runtimeId', $data) && $data['runtimeId'] !== null) {
            $object->setRuntimeId($data['runtimeId']);
        }
        elseif (\array_key_exists('runtimeId', $data) && $data['runtimeId'] === null) {
            $object->setRuntimeId(null);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
        }
        if (\array_key_exists('windowId', $data) && $data['windowId'] !== null) {
            $object->setWindowId($data['windowId']);
        }
        elseif (\array_key_exists('windowId', $data) && $data['windowId'] === null) {
            $object->setWindowId(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['at'] = $data->getAt()->format('Y-m-d\TH:i:sP');
        $dataArray['captureId'] = $data->getCaptureId();
        $dataArray['data'] = $data->getData();
        $dataArray['id'] = $data->getId();
        $dataArray['kind'] = $data->getKind();
        $dataArray['level'] = $data->getLevel();
        $dataArray['message'] = $data->getMessage();
        if ($data->isInitialized('redacted') && null !== $data->getRedacted()) {
            $dataArray['redacted'] = $data->getRedacted();
        }
        $dataArray['runtimeId'] = $data->getRuntimeId();
        $dataArray['version'] = $data->getVersion();
        $dataArray['windowId'] = $data->getWindowId();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionTraceRecordResponse::class => false];
    }
}