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
class SyncSubscriptionItemNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\SyncSubscriptionItem::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\SyncSubscriptionItem::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\SyncSubscriptionItem();
        if (\array_key_exists('clientScoped', $data) && \is_int($data['clientScoped'])) {
            $data['clientScoped'] = (bool) $data['clientScoped'];
        }
        if (\array_key_exists('dataOnly', $data) && \is_int($data['dataOnly'])) {
            $data['dataOnly'] = (bool) $data['dataOnly'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
            unset($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
            unset($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
            unset($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('clientScoped', $data) && $data['clientScoped'] !== null) {
            $object->setClientScoped($data['clientScoped']);
            unset($data['clientScoped']);
        }
        elseif (\array_key_exists('clientScoped', $data) && $data['clientScoped'] === null) {
            $object->setClientScoped(null);
        }
        if (\array_key_exists('eventTypes', $data) && $data['eventTypes'] !== null) {
            $values = [];
            foreach ($data['eventTypes'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\EventTypeBindingItem::class, 'json', $context);
            }
            $object->setEventTypes($values);
            unset($data['eventTypes']);
        }
        elseif (\array_key_exists('eventTypes', $data) && $data['eventTypes'] === null) {
            $object->setEventTypes(null);
        }
        if (\array_key_exists('target', $data) && $data['target'] !== null) {
            $object->setTarget($data['target']);
            unset($data['target']);
        }
        elseif (\array_key_exists('target', $data) && $data['target'] === null) {
            $object->setTarget(null);
        }
        if (\array_key_exists('queue', $data) && $data['queue'] !== null) {
            $object->setQueue($data['queue']);
            unset($data['queue']);
        }
        elseif (\array_key_exists('queue', $data) && $data['queue'] === null) {
            $object->setQueue(null);
        }
        if (\array_key_exists('customConfig', $data) && $data['customConfig'] !== null) {
            $values_1 = [];
            foreach ($data['customConfig'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\ConfigEntry::class, 'json', $context);
            }
            $object->setCustomConfig($values_1);
            unset($data['customConfig']);
        }
        elseif (\array_key_exists('customConfig', $data) && $data['customConfig'] === null) {
            $object->setCustomConfig(null);
        }
        if (\array_key_exists('maxAgeSeconds', $data) && $data['maxAgeSeconds'] !== null) {
            $object->setMaxAgeSeconds($data['maxAgeSeconds']);
            unset($data['maxAgeSeconds']);
        }
        elseif (\array_key_exists('maxAgeSeconds', $data) && $data['maxAgeSeconds'] === null) {
            $object->setMaxAgeSeconds(null);
        }
        if (\array_key_exists('dispatchPoolCode', $data) && $data['dispatchPoolCode'] !== null) {
            $object->setDispatchPoolCode($data['dispatchPoolCode']);
            unset($data['dispatchPoolCode']);
        }
        elseif (\array_key_exists('dispatchPoolCode', $data) && $data['dispatchPoolCode'] === null) {
            $object->setDispatchPoolCode(null);
        }
        if (\array_key_exists('delaySeconds', $data) && $data['delaySeconds'] !== null) {
            $object->setDelaySeconds($data['delaySeconds']);
            unset($data['delaySeconds']);
        }
        elseif (\array_key_exists('delaySeconds', $data) && $data['delaySeconds'] === null) {
            $object->setDelaySeconds(null);
        }
        if (\array_key_exists('sequence', $data) && $data['sequence'] !== null) {
            $object->setSequence($data['sequence']);
            unset($data['sequence']);
        }
        elseif (\array_key_exists('sequence', $data) && $data['sequence'] === null) {
            $object->setSequence(null);
        }
        if (\array_key_exists('mode', $data) && $data['mode'] !== null) {
            $object->setMode($data['mode']);
            unset($data['mode']);
        }
        elseif (\array_key_exists('mode', $data) && $data['mode'] === null) {
            $object->setMode(null);
        }
        if (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] !== null) {
            $object->setTimeoutSeconds($data['timeoutSeconds']);
            unset($data['timeoutSeconds']);
        }
        elseif (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] === null) {
            $object->setTimeoutSeconds(null);
        }
        if (\array_key_exists('maxRetries', $data) && $data['maxRetries'] !== null) {
            $object->setMaxRetries($data['maxRetries']);
            unset($data['maxRetries']);
        }
        elseif (\array_key_exists('maxRetries', $data) && $data['maxRetries'] === null) {
            $object->setMaxRetries(null);
        }
        if (\array_key_exists('dataOnly', $data) && $data['dataOnly'] !== null) {
            $object->setDataOnly($data['dataOnly']);
            unset($data['dataOnly']);
        }
        elseif (\array_key_exists('dataOnly', $data) && $data['dataOnly'] === null) {
            $object->setDataOnly(null);
        }
        foreach ($data as $key => $value_2) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_2;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('code') && null !== $data->getCode()) {
            $dataArray['code'] = $data->getCode();
        }
        if ($data->isInitialized('name') && null !== $data->getName()) {
            $dataArray['name'] = $data->getName();
        }
        if ($data->isInitialized('description') && null !== $data->getDescription()) {
            $dataArray['description'] = $data->getDescription();
        }
        if ($data->isInitialized('clientScoped') && null !== $data->getClientScoped()) {
            $dataArray['clientScoped'] = $data->getClientScoped();
        }
        if ($data->isInitialized('eventTypes') && null !== $data->getEventTypes()) {
            $values = [];
            foreach ($data->getEventTypes() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['eventTypes'] = $values;
        }
        if ($data->isInitialized('target') && null !== $data->getTarget()) {
            $dataArray['target'] = $data->getTarget();
        }
        if ($data->isInitialized('queue') && null !== $data->getQueue()) {
            $dataArray['queue'] = $data->getQueue();
        }
        if ($data->isInitialized('customConfig') && null !== $data->getCustomConfig()) {
            $values_1 = [];
            foreach ($data->getCustomConfig() as $value_1) {
                $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
            }
            $dataArray['customConfig'] = $values_1;
        }
        if ($data->isInitialized('maxAgeSeconds') && null !== $data->getMaxAgeSeconds()) {
            $dataArray['maxAgeSeconds'] = $data->getMaxAgeSeconds();
        }
        if ($data->isInitialized('dispatchPoolCode') && null !== $data->getDispatchPoolCode()) {
            $dataArray['dispatchPoolCode'] = $data->getDispatchPoolCode();
        }
        if ($data->isInitialized('delaySeconds') && null !== $data->getDelaySeconds()) {
            $dataArray['delaySeconds'] = $data->getDelaySeconds();
        }
        if ($data->isInitialized('sequence') && null !== $data->getSequence()) {
            $dataArray['sequence'] = $data->getSequence();
        }
        if ($data->isInitialized('mode') && null !== $data->getMode()) {
            $dataArray['mode'] = $data->getMode();
        }
        if ($data->isInitialized('timeoutSeconds') && null !== $data->getTimeoutSeconds()) {
            $dataArray['timeoutSeconds'] = $data->getTimeoutSeconds();
        }
        if ($data->isInitialized('maxRetries') && null !== $data->getMaxRetries()) {
            $dataArray['maxRetries'] = $data->getMaxRetries();
        }
        if ($data->isInitialized('dataOnly') && null !== $data->getDataOnly()) {
            $dataArray['dataOnly'] = $data->getDataOnly();
        }
        foreach ($data as $key => $value_2) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_2;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\SyncSubscriptionItem::class => false];
    }
}