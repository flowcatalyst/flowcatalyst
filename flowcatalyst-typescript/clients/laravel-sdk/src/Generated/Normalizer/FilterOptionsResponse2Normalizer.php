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
class FilterOptionsResponse2Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FilterOptionsResponse2::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FilterOptionsResponse2::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\FilterOptionsResponse2();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('clients', $data) && $data['clients'] !== null) {
            $values = [];
            foreach ($data['clients'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\FilterOption1::class, 'json', $context);
            }
            $object->setClients($values);
            unset($data['clients']);
        }
        elseif (\array_key_exists('clients', $data) && $data['clients'] === null) {
            $object->setClients(null);
        }
        if (\array_key_exists('applications', $data) && $data['applications'] !== null) {
            $values_1 = [];
            foreach ($data['applications'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\FilterOption1::class, 'json', $context);
            }
            $object->setApplications($values_1);
            unset($data['applications']);
        }
        elseif (\array_key_exists('applications', $data) && $data['applications'] === null) {
            $object->setApplications(null);
        }
        if (\array_key_exists('subdomains', $data) && $data['subdomains'] !== null) {
            $values_2 = [];
            foreach ($data['subdomains'] as $value_2) {
                $values_2[] = $this->denormalizer->denormalize($value_2, \FlowCatalyst\Generated\Model\FilterOption1::class, 'json', $context);
            }
            $object->setSubdomains($values_2);
            unset($data['subdomains']);
        }
        elseif (\array_key_exists('subdomains', $data) && $data['subdomains'] === null) {
            $object->setSubdomains(null);
        }
        if (\array_key_exists('aggregates', $data) && $data['aggregates'] !== null) {
            $values_3 = [];
            foreach ($data['aggregates'] as $value_3) {
                $values_3[] = $this->denormalizer->denormalize($value_3, \FlowCatalyst\Generated\Model\FilterOption1::class, 'json', $context);
            }
            $object->setAggregates($values_3);
            unset($data['aggregates']);
        }
        elseif (\array_key_exists('aggregates', $data) && $data['aggregates'] === null) {
            $object->setAggregates(null);
        }
        if (\array_key_exists('types', $data) && $data['types'] !== null) {
            $values_4 = [];
            foreach ($data['types'] as $value_4) {
                $values_4[] = $this->denormalizer->denormalize($value_4, \FlowCatalyst\Generated\Model\FilterOption1::class, 'json', $context);
            }
            $object->setTypes($values_4);
            unset($data['types']);
        }
        elseif (\array_key_exists('types', $data) && $data['types'] === null) {
            $object->setTypes(null);
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
        if ($data->isInitialized('clients') && null !== $data->getClients()) {
            $values = [];
            foreach ($data->getClients() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['clients'] = $values;
        }
        if ($data->isInitialized('applications') && null !== $data->getApplications()) {
            $values_1 = [];
            foreach ($data->getApplications() as $value_1) {
                $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
            }
            $dataArray['applications'] = $values_1;
        }
        if ($data->isInitialized('subdomains') && null !== $data->getSubdomains()) {
            $values_2 = [];
            foreach ($data->getSubdomains() as $value_2) {
                $values_2[] = $this->normalizer->normalize($value_2, 'json', $context);
            }
            $dataArray['subdomains'] = $values_2;
        }
        if ($data->isInitialized('aggregates') && null !== $data->getAggregates()) {
            $values_3 = [];
            foreach ($data->getAggregates() as $value_3) {
                $values_3[] = $this->normalizer->normalize($value_3, 'json', $context);
            }
            $dataArray['aggregates'] = $values_3;
        }
        if ($data->isInitialized('types') && null !== $data->getTypes()) {
            $values_4 = [];
            foreach ($data->getTypes() as $value_4) {
                $values_4[] = $this->normalizer->normalize($value_4, 'json', $context);
            }
            $dataArray['types'] = $values_4;
        }
        foreach ($data as $key => $value_5) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_5;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FilterOptionsResponse2::class => false];
    }
}