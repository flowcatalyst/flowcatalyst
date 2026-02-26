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
class ApiAdminSubscriptionsIdGetResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200();
        if (\array_key_exists('clientScoped', $data) && \is_int($data['clientScoped'])) {
            $data['clientScoped'] = (bool) $data['clientScoped'];
        }
        if (\array_key_exists('dataOnly', $data) && \is_int($data['dataOnly'])) {
            $data['dataOnly'] = (bool) $data['dataOnly'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
            unset($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
            unset($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
            unset($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
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
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
            unset($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('clientIdentifier', $data) && $data['clientIdentifier'] !== null) {
            $object->setClientIdentifier($data['clientIdentifier']);
            unset($data['clientIdentifier']);
        }
        elseif (\array_key_exists('clientIdentifier', $data) && $data['clientIdentifier'] === null) {
            $object->setClientIdentifier(null);
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
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200EventTypesItem::class, 'json', $context);
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
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200CustomConfigItem::class, 'json', $context);
            }
            $object->setCustomConfig($values_1);
            unset($data['customConfig']);
        }
        elseif (\array_key_exists('customConfig', $data) && $data['customConfig'] === null) {
            $object->setCustomConfig(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
            unset($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
            unset($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('maxAgeSeconds', $data) && $data['maxAgeSeconds'] !== null) {
            $object->setMaxAgeSeconds($data['maxAgeSeconds']);
            unset($data['maxAgeSeconds']);
        }
        elseif (\array_key_exists('maxAgeSeconds', $data) && $data['maxAgeSeconds'] === null) {
            $object->setMaxAgeSeconds(null);
        }
        if (\array_key_exists('dispatchPoolId', $data) && $data['dispatchPoolId'] !== null) {
            $object->setDispatchPoolId($data['dispatchPoolId']);
            unset($data['dispatchPoolId']);
        }
        elseif (\array_key_exists('dispatchPoolId', $data) && $data['dispatchPoolId'] === null) {
            $object->setDispatchPoolId(null);
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
        if (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] !== null) {
            $object->setServiceAccountId($data['serviceAccountId']);
            unset($data['serviceAccountId']);
        }
        elseif (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] === null) {
            $object->setServiceAccountId(null);
        }
        if (\array_key_exists('dataOnly', $data) && $data['dataOnly'] !== null) {
            $object->setDataOnly($data['dataOnly']);
            unset($data['dataOnly']);
        }
        elseif (\array_key_exists('dataOnly', $data) && $data['dataOnly'] === null) {
            $object->setDataOnly(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
            unset($data['updatedAt']);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
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
        $dataArray['id'] = $data->getId();
        $dataArray['code'] = $data->getCode();
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['name'] = $data->getName();
        $dataArray['description'] = $data->getDescription();
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['clientIdentifier'] = $data->getClientIdentifier();
        $dataArray['clientScoped'] = $data->getClientScoped();
        $values = [];
        foreach ($data->getEventTypes() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['eventTypes'] = $values;
        $dataArray['target'] = $data->getTarget();
        $dataArray['queue'] = $data->getQueue();
        $values_1 = [];
        foreach ($data->getCustomConfig() as $value_1) {
            $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
        }
        $dataArray['customConfig'] = $values_1;
        $dataArray['source'] = $data->getSource();
        $dataArray['status'] = $data->getStatus();
        $dataArray['maxAgeSeconds'] = $data->getMaxAgeSeconds();
        $dataArray['dispatchPoolId'] = $data->getDispatchPoolId();
        $dataArray['dispatchPoolCode'] = $data->getDispatchPoolCode();
        $dataArray['delaySeconds'] = $data->getDelaySeconds();
        $dataArray['sequence'] = $data->getSequence();
        $dataArray['mode'] = $data->getMode();
        $dataArray['timeoutSeconds'] = $data->getTimeoutSeconds();
        $dataArray['maxRetries'] = $data->getMaxRetries();
        $dataArray['serviceAccountId'] = $data->getServiceAccountId();
        $dataArray['dataOnly'] = $data->getDataOnly();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value_2) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_2;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200::class => false];
    }
}