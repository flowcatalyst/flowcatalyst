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
class ReleaseRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ReleaseRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ReleaseRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\ReleaseRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('goodbye', $data) && \is_int($data['goodbye'])) {
            $data['goodbye'] = (bool) $data['goodbye'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('goodbye', $data) && $data['goodbye'] !== null) {
            $object->setGoodbye($data['goodbye']);
            unset($data['goodbye']);
        }
        elseif (\array_key_exists('goodbye', $data) && $data['goodbye'] === null) {
            $object->setGoodbye(null);
        }
        if (\array_key_exists('ingestionIds', $data) && $data['ingestionIds'] !== null) {
            $values = [];
            foreach ($data['ingestionIds'] as $value) {
                $values[] = $value;
            }
            $object->setIngestionIds($values);
            unset($data['ingestionIds']);
        }
        elseif (\array_key_exists('ingestionIds', $data) && $data['ingestionIds'] === null) {
            $object->setIngestionIds(null);
        }
        if (\array_key_exists('instanceId', $data) && $data['instanceId'] !== null) {
            $object->setInstanceId($data['instanceId']);
            unset($data['instanceId']);
        }
        elseif (\array_key_exists('instanceId', $data) && $data['instanceId'] === null) {
            $object->setInstanceId(null);
        }
        if (\array_key_exists('runtimeId', $data) && $data['runtimeId'] !== null) {
            $object->setRuntimeId($data['runtimeId']);
            unset($data['runtimeId']);
        }
        elseif (\array_key_exists('runtimeId', $data) && $data['runtimeId'] === null) {
            $object->setRuntimeId(null);
        }
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_1;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['goodbye'] = $data->getGoodbye();
        $values = [];
        foreach ($data->getIngestionIds() as $value) {
            $values[] = $value;
        }
        $dataArray['ingestionIds'] = $values;
        if ($data->isInitialized('instanceId') && null !== $data->getInstanceId()) {
            $dataArray['instanceId'] = $data->getInstanceId();
        }
        $dataArray['runtimeId'] = $data->getRuntimeId();
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ReleaseRequest::class => false];
    }
}