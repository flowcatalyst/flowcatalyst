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
class EnableTraceRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\EnableTraceRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\EnableTraceRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\EnableTraceRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('indefinite', $data) && \is_int($data['indefinite'])) {
            $data['indefinite'] = (bool) $data['indefinite'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('indefinite', $data) && $data['indefinite'] !== null) {
            $object->setIndefinite($data['indefinite']);
            unset($data['indefinite']);
        }
        elseif (\array_key_exists('indefinite', $data) && $data['indefinite'] === null) {
            $object->setIndefinite(null);
        }
        if (\array_key_exists('minutes', $data) && $data['minutes'] !== null) {
            $object->setMinutes($data['minutes']);
            unset($data['minutes']);
        }
        elseif (\array_key_exists('minutes', $data) && $data['minutes'] === null) {
            $object->setMinutes(null);
        }
        if (\array_key_exists('sampleRate', $data) && $data['sampleRate'] !== null) {
            $object->setSampleRate($data['sampleRate']);
            unset($data['sampleRate']);
        }
        elseif (\array_key_exists('sampleRate', $data) && $data['sampleRate'] === null) {
            $object->setSampleRate(null);
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
        if ($data->isInitialized('indefinite') && null !== $data->getIndefinite()) {
            $dataArray['indefinite'] = $data->getIndefinite();
        }
        if ($data->isInitialized('minutes') && null !== $data->getMinutes()) {
            $dataArray['minutes'] = $data->getMinutes();
        }
        if ($data->isInitialized('sampleRate') && null !== $data->getSampleRate()) {
            $dataArray['sampleRate'] = $data->getSampleRate();
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\EnableTraceRequest::class => false];
    }
}