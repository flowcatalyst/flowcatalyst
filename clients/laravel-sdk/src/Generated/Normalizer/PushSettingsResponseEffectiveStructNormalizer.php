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
class PushSettingsResponseEffectiveStructNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PushSettingsResponseEffectiveStruct::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PushSettingsResponseEffectiveStruct::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PushSettingsResponseEffectiveStruct();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('maxDataBytes', $data) && $data['maxDataBytes'] !== null) {
            $object->setMaxDataBytes($data['maxDataBytes']);
        }
        elseif (\array_key_exists('maxDataBytes', $data) && $data['maxDataBytes'] === null) {
            $object->setMaxDataBytes(null);
        }
        if (\array_key_exists('maxStreams', $data) && $data['maxStreams'] !== null) {
            $object->setMaxStreams($data['maxStreams']);
        }
        elseif (\array_key_exists('maxStreams', $data) && $data['maxStreams'] === null) {
            $object->setMaxStreams(null);
        }
        if (\array_key_exists('publishBurst', $data) && $data['publishBurst'] !== null) {
            $object->setPublishBurst($data['publishBurst']);
        }
        elseif (\array_key_exists('publishBurst', $data) && $data['publishBurst'] === null) {
            $object->setPublishBurst(null);
        }
        if (\array_key_exists('publishRate', $data) && $data['publishRate'] !== null) {
            $object->setPublishRate($data['publishRate']);
        }
        elseif (\array_key_exists('publishRate', $data) && $data['publishRate'] === null) {
            $object->setPublishRate(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['maxDataBytes'] = $data->getMaxDataBytes();
        $dataArray['maxStreams'] = $data->getMaxStreams();
        $dataArray['publishBurst'] = $data->getPublishBurst();
        $dataArray['publishRate'] = $data->getPublishRate();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PushSettingsResponseEffectiveStruct::class => false];
    }
}