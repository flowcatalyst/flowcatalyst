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
class BffEventsFilterOptionsGetResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\BffEventsFilterOptionsGetResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\BffEventsFilterOptionsGetResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\BffEventsFilterOptionsGetResponse200();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('applications', $data) && $data['applications'] !== null) {
            $values = [];
            foreach ($data['applications'] as $value) {
                $values[] = $value;
            }
            $object->setApplications($values);
            unset($data['applications']);
        }
        elseif (\array_key_exists('applications', $data) && $data['applications'] === null) {
            $object->setApplications(null);
        }
        if (\array_key_exists('subdomains', $data) && $data['subdomains'] !== null) {
            $values_1 = [];
            foreach ($data['subdomains'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setSubdomains($values_1);
            unset($data['subdomains']);
        }
        elseif (\array_key_exists('subdomains', $data) && $data['subdomains'] === null) {
            $object->setSubdomains(null);
        }
        if (\array_key_exists('aggregates', $data) && $data['aggregates'] !== null) {
            $values_2 = [];
            foreach ($data['aggregates'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setAggregates($values_2);
            unset($data['aggregates']);
        }
        elseif (\array_key_exists('aggregates', $data) && $data['aggregates'] === null) {
            $object->setAggregates(null);
        }
        if (\array_key_exists('types', $data) && $data['types'] !== null) {
            $values_3 = [];
            foreach ($data['types'] as $value_3) {
                $values_3[] = $value_3;
            }
            $object->setTypes($values_3);
            unset($data['types']);
        }
        elseif (\array_key_exists('types', $data) && $data['types'] === null) {
            $object->setTypes(null);
        }
        foreach ($data as $key => $value_4) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_4;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getApplications() as $value) {
            $values[] = $value;
        }
        $dataArray['applications'] = $values;
        $values_1 = [];
        foreach ($data->getSubdomains() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['subdomains'] = $values_1;
        $values_2 = [];
        foreach ($data->getAggregates() as $value_2) {
            $values_2[] = $value_2;
        }
        $dataArray['aggregates'] = $values_2;
        $values_3 = [];
        foreach ($data->getTypes() as $value_3) {
            $values_3[] = $value_3;
        }
        $dataArray['types'] = $values_3;
        foreach ($data as $key => $value_4) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_4;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\BffEventsFilterOptionsGetResponse200::class => false];
    }
}