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
class CircuitBreakersResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\CircuitBreakersResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\CircuitBreakersResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\CircuitBreakersResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('breakers', $data) && $data['breakers'] !== null) {
            $values = [];
            foreach ($data['breakers'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\CircuitBreakerState::class, 'json', $context);
            }
            $object->setBreakers($values);
            unset($data['breakers']);
        }
        elseif (\array_key_exists('breakers', $data) && $data['breakers'] === null) {
            $object->setBreakers(null);
        }
        if (\array_key_exists('totalClosed', $data) && $data['totalClosed'] !== null) {
            $object->setTotalClosed($data['totalClosed']);
            unset($data['totalClosed']);
        }
        elseif (\array_key_exists('totalClosed', $data) && $data['totalClosed'] === null) {
            $object->setTotalClosed(null);
        }
        if (\array_key_exists('totalHalfOpen', $data) && $data['totalHalfOpen'] !== null) {
            $object->setTotalHalfOpen($data['totalHalfOpen']);
            unset($data['totalHalfOpen']);
        }
        elseif (\array_key_exists('totalHalfOpen', $data) && $data['totalHalfOpen'] === null) {
            $object->setTotalHalfOpen(null);
        }
        if (\array_key_exists('totalOpen', $data) && $data['totalOpen'] !== null) {
            $object->setTotalOpen($data['totalOpen']);
            unset($data['totalOpen']);
        }
        elseif (\array_key_exists('totalOpen', $data) && $data['totalOpen'] === null) {
            $object->setTotalOpen(null);
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
        $values = [];
        foreach ($data->getBreakers() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['breakers'] = $values;
        $dataArray['totalClosed'] = $data->getTotalClosed();
        $dataArray['totalHalfOpen'] = $data->getTotalHalfOpen();
        $dataArray['totalOpen'] = $data->getTotalOpen();
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\CircuitBreakersResponse::class => false];
    }
}