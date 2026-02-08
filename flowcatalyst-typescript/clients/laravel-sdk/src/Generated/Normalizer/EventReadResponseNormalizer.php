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
class EventReadResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\EventReadResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\EventReadResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\EventReadResponse();
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
        if (\array_key_exists('specVersion', $data) && $data['specVersion'] !== null) {
            $object->setSpecVersion($data['specVersion']);
            unset($data['specVersion']);
        }
        elseif (\array_key_exists('specVersion', $data) && $data['specVersion'] === null) {
            $object->setSpecVersion(null);
        }
        if (\array_key_exists('type', $data) && $data['type'] !== null) {
            $object->setType($data['type']);
            unset($data['type']);
        }
        elseif (\array_key_exists('type', $data) && $data['type'] === null) {
            $object->setType(null);
        }
        if (\array_key_exists('application', $data) && $data['application'] !== null) {
            $object->setApplication($data['application']);
            unset($data['application']);
        }
        elseif (\array_key_exists('application', $data) && $data['application'] === null) {
            $object->setApplication(null);
        }
        if (\array_key_exists('subdomain', $data) && $data['subdomain'] !== null) {
            $object->setSubdomain($data['subdomain']);
            unset($data['subdomain']);
        }
        elseif (\array_key_exists('subdomain', $data) && $data['subdomain'] === null) {
            $object->setSubdomain(null);
        }
        if (\array_key_exists('aggregate', $data) && $data['aggregate'] !== null) {
            $object->setAggregate($data['aggregate']);
            unset($data['aggregate']);
        }
        elseif (\array_key_exists('aggregate', $data) && $data['aggregate'] === null) {
            $object->setAggregate(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
            unset($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('subject', $data) && $data['subject'] !== null) {
            $object->setSubject($data['subject']);
            unset($data['subject']);
        }
        elseif (\array_key_exists('subject', $data) && $data['subject'] === null) {
            $object->setSubject(null);
        }
        if (\array_key_exists('time', $data) && $data['time'] !== null) {
            $object->setTime($data['time']);
            unset($data['time']);
        }
        elseif (\array_key_exists('time', $data) && $data['time'] === null) {
            $object->setTime(null);
        }
        if (\array_key_exists('data', $data) && $data['data'] !== null) {
            $object->setData($data['data']);
            unset($data['data']);
        }
        elseif (\array_key_exists('data', $data) && $data['data'] === null) {
            $object->setData(null);
        }
        if (\array_key_exists('messageGroup', $data) && $data['messageGroup'] !== null) {
            $object->setMessageGroup($data['messageGroup']);
            unset($data['messageGroup']);
        }
        elseif (\array_key_exists('messageGroup', $data) && $data['messageGroup'] === null) {
            $object->setMessageGroup(null);
        }
        if (\array_key_exists('correlationId', $data) && $data['correlationId'] !== null) {
            $object->setCorrelationId($data['correlationId']);
            unset($data['correlationId']);
        }
        elseif (\array_key_exists('correlationId', $data) && $data['correlationId'] === null) {
            $object->setCorrelationId(null);
        }
        if (\array_key_exists('causationId', $data) && $data['causationId'] !== null) {
            $object->setCausationId($data['causationId']);
            unset($data['causationId']);
        }
        elseif (\array_key_exists('causationId', $data) && $data['causationId'] === null) {
            $object->setCausationId(null);
        }
        if (\array_key_exists('deduplicationId', $data) && $data['deduplicationId'] !== null) {
            $object->setDeduplicationId($data['deduplicationId']);
            unset($data['deduplicationId']);
        }
        elseif (\array_key_exists('deduplicationId', $data) && $data['deduplicationId'] === null) {
            $object->setDeduplicationId(null);
        }
        if (\array_key_exists('contextData', $data) && $data['contextData'] !== null) {
            $values = [];
            foreach ($data['contextData'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\ContextDataResponse1::class, 'json', $context);
            }
            $object->setContextData($values);
            unset($data['contextData']);
        }
        elseif (\array_key_exists('contextData', $data) && $data['contextData'] === null) {
            $object->setContextData(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
            unset($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('projectedAt', $data) && $data['projectedAt'] !== null) {
            $object->setProjectedAt($data['projectedAt']);
            unset($data['projectedAt']);
        }
        elseif (\array_key_exists('projectedAt', $data) && $data['projectedAt'] === null) {
            $object->setProjectedAt(null);
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
        if ($data->isInitialized('id') && null !== $data->getId()) {
            $dataArray['id'] = $data->getId();
        }
        if ($data->isInitialized('specVersion') && null !== $data->getSpecVersion()) {
            $dataArray['specVersion'] = $data->getSpecVersion();
        }
        if ($data->isInitialized('type') && null !== $data->getType()) {
            $dataArray['type'] = $data->getType();
        }
        if ($data->isInitialized('application') && null !== $data->getApplication()) {
            $dataArray['application'] = $data->getApplication();
        }
        if ($data->isInitialized('subdomain') && null !== $data->getSubdomain()) {
            $dataArray['subdomain'] = $data->getSubdomain();
        }
        if ($data->isInitialized('aggregate') && null !== $data->getAggregate()) {
            $dataArray['aggregate'] = $data->getAggregate();
        }
        if ($data->isInitialized('source') && null !== $data->getSource()) {
            $dataArray['source'] = $data->getSource();
        }
        if ($data->isInitialized('subject') && null !== $data->getSubject()) {
            $dataArray['subject'] = $data->getSubject();
        }
        if ($data->isInitialized('time') && null !== $data->getTime()) {
            $dataArray['time'] = $data->getTime();
        }
        if ($data->isInitialized('data') && null !== $data->getData()) {
            $dataArray['data'] = $data->getData();
        }
        if ($data->isInitialized('messageGroup') && null !== $data->getMessageGroup()) {
            $dataArray['messageGroup'] = $data->getMessageGroup();
        }
        if ($data->isInitialized('correlationId') && null !== $data->getCorrelationId()) {
            $dataArray['correlationId'] = $data->getCorrelationId();
        }
        if ($data->isInitialized('causationId') && null !== $data->getCausationId()) {
            $dataArray['causationId'] = $data->getCausationId();
        }
        if ($data->isInitialized('deduplicationId') && null !== $data->getDeduplicationId()) {
            $dataArray['deduplicationId'] = $data->getDeduplicationId();
        }
        if ($data->isInitialized('contextData') && null !== $data->getContextData()) {
            $values = [];
            foreach ($data->getContextData() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['contextData'] = $values;
        }
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['clientId'] = $data->getClientId();
        }
        if ($data->isInitialized('projectedAt') && null !== $data->getProjectedAt()) {
            $dataArray['projectedAt'] = $data->getProjectedAt();
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
        return [\FlowCatalyst\Generated\Model\EventReadResponse::class => false];
    }
}