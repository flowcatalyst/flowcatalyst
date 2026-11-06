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
class IngestionTraceRecordNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionTraceRecord::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionTraceRecord::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionTraceRecord();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('at', $data) && $data['at'] !== null) {
            $object->setAt($data['at']);
            unset($data['at']);
        }
        elseif (\array_key_exists('at', $data) && $data['at'] === null) {
            $object->setAt(null);
            unset($data['at']);
        }
        if (\array_key_exists('captureId', $data) && $data['captureId'] !== null) {
            $object->setCaptureId($data['captureId']);
            unset($data['captureId']);
        }
        elseif (\array_key_exists('captureId', $data) && $data['captureId'] === null) {
            $object->setCaptureId(null);
            unset($data['captureId']);
        }
        if (\array_key_exists('data', $data) && $data['data'] !== null) {
            $object->setData($data['data']);
            unset($data['data']);
        }
        elseif (\array_key_exists('data', $data) && $data['data'] === null) {
            $object->setData(null);
            unset($data['data']);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
            unset($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
            unset($data['id']);
        }
        if (\array_key_exists('ingestionId', $data) && $data['ingestionId'] !== null) {
            $object->setIngestionId($data['ingestionId']);
            unset($data['ingestionId']);
        }
        elseif (\array_key_exists('ingestionId', $data) && $data['ingestionId'] === null) {
            $object->setIngestionId(null);
            unset($data['ingestionId']);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
            unset($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
            unset($data['kind']);
        }
        if (\array_key_exists('level', $data) && $data['level'] !== null) {
            $object->setLevel($data['level']);
            unset($data['level']);
        }
        elseif (\array_key_exists('level', $data) && $data['level'] === null) {
            $object->setLevel(null);
            unset($data['level']);
        }
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
            unset($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
            unset($data['message']);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
            unset($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
            unset($data['version']);
        }
        if (\array_key_exists('windowId', $data) && $data['windowId'] !== null) {
            $object->setWindowId($data['windowId']);
            unset($data['windowId']);
        }
        elseif (\array_key_exists('windowId', $data) && $data['windowId'] === null) {
            $object->setWindowId(null);
            unset($data['windowId']);
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['at'] = $data->getAt();
        $dataArray['captureId'] = $data->getCaptureId();
        if ($data->isInitialized('data') && null !== $data->getData()) {
            $dataArray['data'] = $data->getData();
        }
        if ($data->isInitialized('id') && null !== $data->getId()) {
            $dataArray['id'] = $data->getId();
        }
        $dataArray['ingestionId'] = $data->getIngestionId();
        $dataArray['kind'] = $data->getKind();
        $dataArray['level'] = $data->getLevel();
        $dataArray['message'] = $data->getMessage();
        $dataArray['version'] = $data->getVersion();
        $dataArray['windowId'] = $data->getWindowId();
        foreach ($data->additionalPropertyEntries() as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionTraceRecord::class => false];
    }
}