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
class DispatchJobFilterOptionsResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\DispatchJobFilterOptionsResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\DispatchJobFilterOptionsResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\DispatchJobFilterOptionsResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('eventTypeCodes', $data) && $data['eventTypeCodes'] !== null) {
            $values = [];
            foreach ($data['eventTypeCodes'] as $value) {
                $values[] = $value;
            }
            $object->setEventTypeCodes($values);
            unset($data['eventTypeCodes']);
        }
        elseif (\array_key_exists('eventTypeCodes', $data) && $data['eventTypeCodes'] === null) {
            $object->setEventTypeCodes(null);
        }
        if (\array_key_exists('modes', $data) && $data['modes'] !== null) {
            $values_1 = [];
            foreach ($data['modes'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setModes($values_1);
            unset($data['modes']);
        }
        elseif (\array_key_exists('modes', $data) && $data['modes'] === null) {
            $object->setModes(null);
        }
        if (\array_key_exists('statuses', $data) && $data['statuses'] !== null) {
            $values_2 = [];
            foreach ($data['statuses'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setStatuses($values_2);
            unset($data['statuses']);
        }
        elseif (\array_key_exists('statuses', $data) && $data['statuses'] === null) {
            $object->setStatuses(null);
        }
        if (\array_key_exists('subscriptionIds', $data) && $data['subscriptionIds'] !== null) {
            $values_3 = [];
            foreach ($data['subscriptionIds'] as $value_3) {
                $values_3[] = $value_3;
            }
            $object->setSubscriptionIds($values_3);
            unset($data['subscriptionIds']);
        }
        elseif (\array_key_exists('subscriptionIds', $data) && $data['subscriptionIds'] === null) {
            $object->setSubscriptionIds(null);
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
        foreach ($data->getEventTypeCodes() as $value) {
            $values[] = $value;
        }
        $dataArray['eventTypeCodes'] = $values;
        $values_1 = [];
        foreach ($data->getModes() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['modes'] = $values_1;
        $values_2 = [];
        foreach ($data->getStatuses() as $value_2) {
            $values_2[] = $value_2;
        }
        $dataArray['statuses'] = $values_2;
        $values_3 = [];
        foreach ($data->getSubscriptionIds() as $value_3) {
            $values_3[] = $value_3;
        }
        $dataArray['subscriptionIds'] = $values_3;
        foreach ($data as $key => $value_4) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_4;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\DispatchJobFilterOptionsResponse::class => false];
    }
}