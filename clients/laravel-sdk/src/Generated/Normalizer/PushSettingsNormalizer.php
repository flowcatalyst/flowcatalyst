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
class PushSettingsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PushSettings::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PushSettings::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PushSettings();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('paused', $data) && \is_int($data['paused'])) {
            $data['paused'] = (bool) $data['paused'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
            unset($data['$schema']);
        }
        if (\array_key_exists('maxDataBytes', $data) && $data['maxDataBytes'] !== null) {
            $object->setMaxDataBytes($data['maxDataBytes']);
            unset($data['maxDataBytes']);
        }
        elseif (\array_key_exists('maxDataBytes', $data) && $data['maxDataBytes'] === null) {
            $object->setMaxDataBytes(null);
            unset($data['maxDataBytes']);
        }
        if (\array_key_exists('maxStreams', $data) && $data['maxStreams'] !== null) {
            $object->setMaxStreams($data['maxStreams']);
            unset($data['maxStreams']);
        }
        elseif (\array_key_exists('maxStreams', $data) && $data['maxStreams'] === null) {
            $object->setMaxStreams(null);
            unset($data['maxStreams']);
        }
        if (\array_key_exists('paused', $data) && $data['paused'] !== null) {
            $object->setPaused($data['paused']);
            unset($data['paused']);
        }
        elseif (\array_key_exists('paused', $data) && $data['paused'] === null) {
            $object->setPaused(null);
            unset($data['paused']);
        }
        if (\array_key_exists('publishBurst', $data) && $data['publishBurst'] !== null) {
            $object->setPublishBurst($data['publishBurst']);
            unset($data['publishBurst']);
        }
        elseif (\array_key_exists('publishBurst', $data) && $data['publishBurst'] === null) {
            $object->setPublishBurst(null);
            unset($data['publishBurst']);
        }
        if (\array_key_exists('publishRate', $data) && $data['publishRate'] !== null) {
            $object->setPublishRate($data['publishRate']);
            unset($data['publishRate']);
        }
        elseif (\array_key_exists('publishRate', $data) && $data['publishRate'] === null) {
            $object->setPublishRate(null);
            unset($data['publishRate']);
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
        if ($data->isInitialized('maxDataBytes') && null !== $data->getMaxDataBytes()) {
            $dataArray['maxDataBytes'] = $data->getMaxDataBytes();
        }
        if ($data->isInitialized('maxStreams') && null !== $data->getMaxStreams()) {
            $dataArray['maxStreams'] = $data->getMaxStreams();
        }
        $dataArray['paused'] = $data->getPaused();
        if ($data->isInitialized('publishBurst') && null !== $data->getPublishBurst()) {
            $dataArray['publishBurst'] = $data->getPublishBurst();
        }
        if ($data->isInitialized('publishRate') && null !== $data->getPublishRate()) {
            $dataArray['publishRate'] = $data->getPublishRate();
        }
        foreach ($data->additionalPropertyEntries() as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PushSettings::class => false];
    }
}