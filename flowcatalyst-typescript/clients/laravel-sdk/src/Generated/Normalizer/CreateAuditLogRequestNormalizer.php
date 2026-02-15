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
class CreateAuditLogRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\CreateAuditLogRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\CreateAuditLogRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\CreateAuditLogRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('entityType', $data) && $data['entityType'] !== null) {
            $object->setEntityType($data['entityType']);
            unset($data['entityType']);
        }
        elseif (\array_key_exists('entityType', $data) && $data['entityType'] === null) {
            $object->setEntityType(null);
        }
        if (\array_key_exists('entityId', $data) && $data['entityId'] !== null) {
            $object->setEntityId($data['entityId']);
            unset($data['entityId']);
        }
        elseif (\array_key_exists('entityId', $data) && $data['entityId'] === null) {
            $object->setEntityId(null);
        }
        if (\array_key_exists('operation', $data) && $data['operation'] !== null) {
            $object->setOperation($data['operation']);
            unset($data['operation']);
        }
        elseif (\array_key_exists('operation', $data) && $data['operation'] === null) {
            $object->setOperation(null);
        }
        if (\array_key_exists('operationData', $data) && $data['operationData'] !== null) {
            $object->setOperationData($data['operationData']);
            unset($data['operationData']);
        }
        elseif (\array_key_exists('operationData', $data) && $data['operationData'] === null) {
            $object->setOperationData(null);
        }
        if (\array_key_exists('principalId', $data) && $data['principalId'] !== null) {
            $object->setPrincipalId($data['principalId']);
            unset($data['principalId']);
        }
        elseif (\array_key_exists('principalId', $data) && $data['principalId'] === null) {
            $object->setPrincipalId(null);
        }
        if (\array_key_exists('performedAt', $data) && $data['performedAt'] !== null) {
            $object->setPerformedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['performedAt']));
            unset($data['performedAt']);
        }
        elseif (\array_key_exists('performedAt', $data) && $data['performedAt'] === null) {
            $object->setPerformedAt(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
            unset($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('correlationId', $data) && $data['correlationId'] !== null) {
            $object->setCorrelationId($data['correlationId']);
            unset($data['correlationId']);
        }
        elseif (\array_key_exists('correlationId', $data) && $data['correlationId'] === null) {
            $object->setCorrelationId(null);
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
        if ($data->isInitialized('entityType') && null !== $data->getEntityType()) {
            $dataArray['entityType'] = $data->getEntityType();
        }
        if ($data->isInitialized('entityId') && null !== $data->getEntityId()) {
            $dataArray['entityId'] = $data->getEntityId();
        }
        if ($data->isInitialized('operation') && null !== $data->getOperation()) {
            $dataArray['operation'] = $data->getOperation();
        }
        if ($data->isInitialized('operationData') && null !== $data->getOperationData()) {
            $dataArray['operationData'] = $data->getOperationData();
        }
        if ($data->isInitialized('principalId') && null !== $data->getPrincipalId()) {
            $dataArray['principalId'] = $data->getPrincipalId();
        }
        if ($data->isInitialized('performedAt') && null !== $data->getPerformedAt()) {
            $dataArray['performedAt'] = $data->getPerformedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('source') && null !== $data->getSource()) {
            $dataArray['source'] = $data->getSource();
        }
        if ($data->isInitialized('correlationId') && null !== $data->getCorrelationId()) {
            $dataArray['correlationId'] = $data->getCorrelationId();
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\CreateAuditLogRequest::class => false];
    }
}