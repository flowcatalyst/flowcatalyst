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
class BffDebugEventsIdGetResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse200();
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
            $object->setTime(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['time']));
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
            $object->setContextData($data['contextData']);
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
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['id'] = $data->getId();
        $dataArray['specVersion'] = $data->getSpecVersion();
        $dataArray['type'] = $data->getType();
        $dataArray['source'] = $data->getSource();
        $dataArray['subject'] = $data->getSubject();
        $dataArray['time'] = $data->getTime()->format('Y-m-d\TH:i:sP');
        $dataArray['data'] = $data->getData();
        $dataArray['messageGroup'] = $data->getMessageGroup();
        $dataArray['correlationId'] = $data->getCorrelationId();
        $dataArray['causationId'] = $data->getCausationId();
        $dataArray['deduplicationId'] = $data->getDeduplicationId();
        $dataArray['contextData'] = $data->getContextData();
        $dataArray['clientId'] = $data->getClientId();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse200::class => false];
    }
}