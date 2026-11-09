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
class SyncPushRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\SyncPushRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\SyncPushRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\SyncPushRequest();
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
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
            unset($data['$schema']);
        }
        if (\array_key_exists('keys', $data) && $data['keys'] !== null) {
            $object->setKeys($this->denormalizer->denormalize($data['keys'], \FlowCatalyst\Generated\Model\SyncPushKeysRequest::class, 'json', $context));
            unset($data['keys']);
        }
        elseif (\array_key_exists('keys', $data) && $data['keys'] === null) {
            $object->setKeys(null);
            unset($data['keys']);
        }
        if (\array_key_exists('rules', $data) && $data['rules'] !== null) {
            $values = [];
            foreach ($data['rules'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\SyncPushRuleRequest::class, 'json', $context);
            }
            $object->setRules($values);
            unset($data['rules']);
        }
        elseif (\array_key_exists('rules', $data) && $data['rules'] === null) {
            $object->setRules(null);
            unset($data['rules']);
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
        $dataArray['keys'] = $data->getKeys() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getKeys(), 'json', $context));
        $values = [];
        foreach ($data->getRules() as $value) {
            $values[] = $value === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value, 'json', $context));
        }
        $dataArray['rules'] = $values;
        foreach ($data->additionalPropertyEntries() as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\SyncPushRequest::class => false];
    }
}