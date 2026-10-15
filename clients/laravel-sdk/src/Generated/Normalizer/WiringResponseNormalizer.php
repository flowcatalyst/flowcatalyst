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
class WiringResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\WiringResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\WiringResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\WiringResponse();
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
        if (\array_key_exists('dispatchPoolCode', $data) && $data['dispatchPoolCode'] !== null) {
            $object->setDispatchPoolCode($data['dispatchPoolCode']);
        }
        elseif (\array_key_exists('dispatchPoolCode', $data) && $data['dispatchPoolCode'] === null) {
            $object->setDispatchPoolCode(null);
        }
        if (\array_key_exists('schedulesCreated', $data) && $data['schedulesCreated'] !== null) {
            $object->setSchedulesCreated($data['schedulesCreated']);
        }
        elseif (\array_key_exists('schedulesCreated', $data) && $data['schedulesCreated'] === null) {
            $object->setSchedulesCreated(null);
        }
        if (\array_key_exists('schedulesDeleted', $data) && $data['schedulesDeleted'] !== null) {
            $object->setSchedulesDeleted($data['schedulesDeleted']);
        }
        elseif (\array_key_exists('schedulesDeleted', $data) && $data['schedulesDeleted'] === null) {
            $object->setSchedulesDeleted(null);
        }
        if (\array_key_exists('schedulesUpdated', $data) && $data['schedulesUpdated'] !== null) {
            $object->setSchedulesUpdated($data['schedulesUpdated']);
        }
        elseif (\array_key_exists('schedulesUpdated', $data) && $data['schedulesUpdated'] === null) {
            $object->setSchedulesUpdated(null);
        }
        if (\array_key_exists('subscriptionsCreated', $data) && $data['subscriptionsCreated'] !== null) {
            $object->setSubscriptionsCreated($data['subscriptionsCreated']);
        }
        elseif (\array_key_exists('subscriptionsCreated', $data) && $data['subscriptionsCreated'] === null) {
            $object->setSubscriptionsCreated(null);
        }
        if (\array_key_exists('subscriptionsDeleted', $data) && $data['subscriptionsDeleted'] !== null) {
            $object->setSubscriptionsDeleted($data['subscriptionsDeleted']);
        }
        elseif (\array_key_exists('subscriptionsDeleted', $data) && $data['subscriptionsDeleted'] === null) {
            $object->setSubscriptionsDeleted(null);
        }
        if (\array_key_exists('subscriptionsUpdated', $data) && $data['subscriptionsUpdated'] !== null) {
            $object->setSubscriptionsUpdated($data['subscriptionsUpdated']);
        }
        elseif (\array_key_exists('subscriptionsUpdated', $data) && $data['subscriptionsUpdated'] === null) {
            $object->setSubscriptionsUpdated(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('dispatchPoolCode') && null !== $data->getDispatchPoolCode()) {
            $dataArray['dispatchPoolCode'] = $data->getDispatchPoolCode();
        }
        if ($data->isInitialized('schedulesCreated') && null !== $data->getSchedulesCreated()) {
            $dataArray['schedulesCreated'] = $data->getSchedulesCreated();
        }
        if ($data->isInitialized('schedulesDeleted') && null !== $data->getSchedulesDeleted()) {
            $dataArray['schedulesDeleted'] = $data->getSchedulesDeleted();
        }
        if ($data->isInitialized('schedulesUpdated') && null !== $data->getSchedulesUpdated()) {
            $dataArray['schedulesUpdated'] = $data->getSchedulesUpdated();
        }
        if ($data->isInitialized('subscriptionsCreated') && null !== $data->getSubscriptionsCreated()) {
            $dataArray['subscriptionsCreated'] = $data->getSubscriptionsCreated();
        }
        if ($data->isInitialized('subscriptionsDeleted') && null !== $data->getSubscriptionsDeleted()) {
            $dataArray['subscriptionsDeleted'] = $data->getSubscriptionsDeleted();
        }
        if ($data->isInitialized('subscriptionsUpdated') && null !== $data->getSubscriptionsUpdated()) {
            $dataArray['subscriptionsUpdated'] = $data->getSubscriptionsUpdated();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\WiringResponse::class => false];
    }
}