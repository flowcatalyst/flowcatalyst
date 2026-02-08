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
class BatchEventResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\BatchEventResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\BatchEventResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\BatchEventResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('events', $data) && $data['events'] !== null) {
            $values = [];
            foreach ($data['events'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\EventResponse::class, 'json', $context);
            }
            $object->setEvents($values);
            unset($data['events']);
        }
        elseif (\array_key_exists('events', $data) && $data['events'] === null) {
            $object->setEvents(null);
        }
        if (\array_key_exists('count', $data) && $data['count'] !== null) {
            $object->setCount($data['count']);
            unset($data['count']);
        }
        elseif (\array_key_exists('count', $data) && $data['count'] === null) {
            $object->setCount(null);
        }
        if (\array_key_exists('dispatchJobCount', $data) && $data['dispatchJobCount'] !== null) {
            $object->setDispatchJobCount($data['dispatchJobCount']);
            unset($data['dispatchJobCount']);
        }
        elseif (\array_key_exists('dispatchJobCount', $data) && $data['dispatchJobCount'] === null) {
            $object->setDispatchJobCount(null);
        }
        if (\array_key_exists('duplicateCount', $data) && $data['duplicateCount'] !== null) {
            $object->setDuplicateCount($data['duplicateCount']);
            unset($data['duplicateCount']);
        }
        elseif (\array_key_exists('duplicateCount', $data) && $data['duplicateCount'] === null) {
            $object->setDuplicateCount(null);
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
        if ($data->isInitialized('events') && null !== $data->getEvents()) {
            $values = [];
            foreach ($data->getEvents() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['events'] = $values;
        }
        if ($data->isInitialized('count') && null !== $data->getCount()) {
            $dataArray['count'] = $data->getCount();
        }
        if ($data->isInitialized('dispatchJobCount') && null !== $data->getDispatchJobCount()) {
            $dataArray['dispatchJobCount'] = $data->getDispatchJobCount();
        }
        if ($data->isInitialized('duplicateCount') && null !== $data->getDuplicateCount()) {
            $dataArray['duplicateCount'] = $data->getDuplicateCount();
        }
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\BatchEventResponse::class => false];
    }
}