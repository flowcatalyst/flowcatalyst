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
class AllFilterOptionsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AllFilterOptions::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AllFilterOptions::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\AllFilterOptions();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('applications', $data) && $data['applications'] !== null) {
            $values = [];
            foreach ($data['applications'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\FilterOption::class, 'json', $context);
            }
            $object->setApplications($values);
            unset($data['applications']);
        }
        elseif (\array_key_exists('applications', $data) && $data['applications'] === null) {
            $object->setApplications(null);
        }
        if (\array_key_exists('clients', $data) && $data['clients'] !== null) {
            $values_1 = [];
            foreach ($data['clients'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\FilterOption::class, 'json', $context);
            }
            $object->setClients($values_1);
            unset($data['clients']);
        }
        elseif (\array_key_exists('clients', $data) && $data['clients'] === null) {
            $object->setClients(null);
        }
        if (\array_key_exists('dispatchPools', $data) && $data['dispatchPools'] !== null) {
            $values_2 = [];
            foreach ($data['dispatchPools'] as $value_2) {
                $values_2[] = $this->denormalizer->denormalize($value_2, \FlowCatalyst\Generated\Model\FilterOption::class, 'json', $context);
            }
            $object->setDispatchPools($values_2);
            unset($data['dispatchPools']);
        }
        elseif (\array_key_exists('dispatchPools', $data) && $data['dispatchPools'] === null) {
            $object->setDispatchPools(null);
        }
        if (\array_key_exists('eventTypes', $data) && $data['eventTypes'] !== null) {
            $values_3 = [];
            foreach ($data['eventTypes'] as $value_3) {
                $values_3[] = $this->denormalizer->denormalize($value_3, \FlowCatalyst\Generated\Model\FilterOption::class, 'json', $context);
            }
            $object->setEventTypes($values_3);
            unset($data['eventTypes']);
        }
        elseif (\array_key_exists('eventTypes', $data) && $data['eventTypes'] === null) {
            $object->setEventTypes(null);
        }
        if (\array_key_exists('subscriptions', $data) && $data['subscriptions'] !== null) {
            $values_4 = [];
            foreach ($data['subscriptions'] as $value_4) {
                $values_4[] = $this->denormalizer->denormalize($value_4, \FlowCatalyst\Generated\Model\FilterOption::class, 'json', $context);
            }
            $object->setSubscriptions($values_4);
            unset($data['subscriptions']);
        }
        elseif (\array_key_exists('subscriptions', $data) && $data['subscriptions'] === null) {
            $object->setSubscriptions(null);
        }
        foreach ($data as $key => $value_5) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_5;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getApplications() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['applications'] = $values;
        $values_1 = [];
        foreach ($data->getClients() as $value_1) {
            $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
        }
        $dataArray['clients'] = $values_1;
        $values_2 = [];
        foreach ($data->getDispatchPools() as $value_2) {
            $values_2[] = $this->normalizer->normalize($value_2, 'json', $context);
        }
        $dataArray['dispatchPools'] = $values_2;
        $values_3 = [];
        foreach ($data->getEventTypes() as $value_3) {
            $values_3[] = $this->normalizer->normalize($value_3, 'json', $context);
        }
        $dataArray['eventTypes'] = $values_3;
        $values_4 = [];
        foreach ($data->getSubscriptions() as $value_4) {
            $values_4[] = $this->normalizer->normalize($value_4, 'json', $context);
        }
        $dataArray['subscriptions'] = $values_4;
        foreach ($data as $key => $value_5) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_5;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AllFilterOptions::class => false];
    }
}