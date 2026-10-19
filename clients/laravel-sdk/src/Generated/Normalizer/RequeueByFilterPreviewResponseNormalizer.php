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
class RequeueByFilterPreviewResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\RequeueByFilterPreviewResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\RequeueByFilterPreviewResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\RequeueByFilterPreviewResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('byStatus', $data) && $data['byStatus'] !== null) {
            $values = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['byStatus'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setByStatus($values);
        }
        elseif (\array_key_exists('byStatus', $data) && $data['byStatus'] === null) {
            $object->setByStatus(null);
        }
        if (\array_key_exists('completedCount', $data) && $data['completedCount'] !== null) {
            $object->setCompletedCount($data['completedCount']);
        }
        elseif (\array_key_exists('completedCount', $data) && $data['completedCount'] === null) {
            $object->setCompletedCount(null);
        }
        if (\array_key_exists('count', $data) && $data['count'] !== null) {
            $object->setCount($data['count']);
        }
        elseif (\array_key_exists('count', $data) && $data['count'] === null) {
            $object->setCount(null);
        }
        if (\array_key_exists('eligibleCount', $data) && $data['eligibleCount'] !== null) {
            $object->setEligibleCount($data['eligibleCount']);
        }
        elseif (\array_key_exists('eligibleCount', $data) && $data['eligibleCount'] === null) {
            $object->setEligibleCount(null);
        }
        if (\array_key_exists('inFlightCount', $data) && $data['inFlightCount'] !== null) {
            $object->setInFlightCount($data['inFlightCount']);
        }
        elseif (\array_key_exists('inFlightCount', $data) && $data['inFlightCount'] === null) {
            $object->setInFlightCount(null);
        }
        if (\array_key_exists('redriveEligibleCount', $data) && $data['redriveEligibleCount'] !== null) {
            $object->setRedriveEligibleCount($data['redriveEligibleCount']);
        }
        elseif (\array_key_exists('redriveEligibleCount', $data) && $data['redriveEligibleCount'] === null) {
            $object->setRedriveEligibleCount(null);
        }
        if (\array_key_exists('window', $data) && $data['window'] !== null) {
            $object->setWindow($this->denormalizer->denormalize($data['window'], \FlowCatalyst\Generated\Model\WindowResponse::class, 'json', $context));
        }
        elseif (\array_key_exists('window', $data) && $data['window'] === null) {
            $object->setWindow(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getByStatus() as $key => $value) {
            $values[$key] = $value;
        }
        $dataArray['byStatus'] = $values;
        $dataArray['completedCount'] = $data->getCompletedCount();
        $dataArray['count'] = $data->getCount();
        $dataArray['eligibleCount'] = $data->getEligibleCount();
        $dataArray['inFlightCount'] = $data->getInFlightCount();
        $dataArray['redriveEligibleCount'] = $data->getRedriveEligibleCount();
        $dataArray['window'] = $this->normalizer->normalize($data->getWindow(), 'json', $context);
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\RequeueByFilterPreviewResponse::class => false];
    }
}