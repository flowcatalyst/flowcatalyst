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
class PushSyncResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PushSyncResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PushSyncResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PushSyncResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('keys', $data) && $data['keys'] !== null) {
            $object->setKeys($this->denormalizer->denormalize($data['keys'], \FlowCatalyst\Generated\Model\PushSyncCounts::class, 'json', $context));
        }
        elseif (\array_key_exists('keys', $data) && $data['keys'] === null) {
            $object->setKeys(null);
        }
        if (\array_key_exists('rules', $data) && $data['rules'] !== null) {
            $object->setRules($this->denormalizer->denormalize($data['rules'], \FlowCatalyst\Generated\Model\PushSyncCounts::class, 'json', $context));
        }
        elseif (\array_key_exists('rules', $data) && $data['rules'] === null) {
            $object->setRules(null);
        }
        if (\array_key_exists('syncedKids', $data) && $data['syncedKids'] !== null) {
            $values = [];
            foreach ($data['syncedKids'] as $value) {
                $values[] = $value;
            }
            $object->setSyncedKids($values);
        }
        elseif (\array_key_exists('syncedKids', $data) && $data['syncedKids'] === null) {
            $object->setSyncedKids(null);
        }
        if (\array_key_exists('syncedPatterns', $data) && $data['syncedPatterns'] !== null) {
            $values_1 = [];
            foreach ($data['syncedPatterns'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setSyncedPatterns($values_1);
        }
        elseif (\array_key_exists('syncedPatterns', $data) && $data['syncedPatterns'] === null) {
            $object->setSyncedPatterns(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['keys'] = $data->getKeys() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getKeys(), 'json', $context));
        $dataArray['rules'] = $data->getRules() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getRules(), 'json', $context));
        $values = [];
        foreach ($data->getSyncedKids() as $value) {
            $values[] = $value;
        }
        $dataArray['syncedKids'] = $values;
        $values_1 = [];
        foreach ($data->getSyncedPatterns() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['syncedPatterns'] = $values_1;
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PushSyncResponse::class => false];
    }
}