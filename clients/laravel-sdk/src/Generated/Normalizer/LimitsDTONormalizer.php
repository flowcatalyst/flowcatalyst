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
class LimitsDTONormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\LimitsDTO::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\LimitsDTO::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\LimitsDTO();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('maxBodyBytes', $data) && $data['maxBodyBytes'] !== null) {
            $object->setMaxBodyBytes($data['maxBodyBytes']);
        }
        elseif (\array_key_exists('maxBodyBytes', $data) && $data['maxBodyBytes'] === null) {
            $object->setMaxBodyBytes(null);
        }
        if (\array_key_exists('maxConcurrency', $data) && $data['maxConcurrency'] !== null) {
            $object->setMaxConcurrency($data['maxConcurrency']);
        }
        elseif (\array_key_exists('maxConcurrency', $data) && $data['maxConcurrency'] === null) {
            $object->setMaxConcurrency(null);
        }
        if (\array_key_exists('memoryMb', $data) && $data['memoryMb'] !== null) {
            $object->setMemoryMb($data['memoryMb']);
        }
        elseif (\array_key_exists('memoryMb', $data) && $data['memoryMb'] === null) {
            $object->setMemoryMb(null);
        }
        if (\array_key_exists('timeoutMs', $data) && $data['timeoutMs'] !== null) {
            $object->setTimeoutMs($data['timeoutMs']);
        }
        elseif (\array_key_exists('timeoutMs', $data) && $data['timeoutMs'] === null) {
            $object->setTimeoutMs(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('maxBodyBytes') && null !== $data->getMaxBodyBytes()) {
            $dataArray['maxBodyBytes'] = $data->getMaxBodyBytes();
        }
        if ($data->isInitialized('maxConcurrency') && null !== $data->getMaxConcurrency()) {
            $dataArray['maxConcurrency'] = $data->getMaxConcurrency();
        }
        if ($data->isInitialized('memoryMb') && null !== $data->getMemoryMb()) {
            $dataArray['memoryMb'] = $data->getMemoryMb();
        }
        if ($data->isInitialized('timeoutMs') && null !== $data->getTimeoutMs()) {
            $dataArray['timeoutMs'] = $data->getTimeoutMs();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\LimitsDTO::class => false];
    }
}