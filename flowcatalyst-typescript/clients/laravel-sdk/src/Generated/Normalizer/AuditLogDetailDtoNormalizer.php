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
class AuditLogDetailDtoNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AuditLogDetailDto::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AuditLogDetailDto::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\AuditLogDetailDto();
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
        if (\array_key_exists('operationJson', $data) && $data['operationJson'] !== null) {
            $object->setOperationJson($data['operationJson']);
            unset($data['operationJson']);
        }
        elseif (\array_key_exists('operationJson', $data) && $data['operationJson'] === null) {
            $object->setOperationJson(null);
        }
        if (\array_key_exists('principalId', $data) && $data['principalId'] !== null) {
            $object->setPrincipalId($data['principalId']);
            unset($data['principalId']);
        }
        elseif (\array_key_exists('principalId', $data) && $data['principalId'] === null) {
            $object->setPrincipalId(null);
        }
        if (\array_key_exists('principalName', $data) && $data['principalName'] !== null) {
            $object->setPrincipalName($data['principalName']);
            unset($data['principalName']);
        }
        elseif (\array_key_exists('principalName', $data) && $data['principalName'] === null) {
            $object->setPrincipalName(null);
        }
        if (\array_key_exists('performedAt', $data) && $data['performedAt'] !== null) {
            $object->setPerformedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['performedAt']));
            unset($data['performedAt']);
        }
        elseif (\array_key_exists('performedAt', $data) && $data['performedAt'] === null) {
            $object->setPerformedAt(null);
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
        if ($data->isInitialized('id') && null !== $data->getId()) {
            $dataArray['id'] = $data->getId();
        }
        if ($data->isInitialized('entityType') && null !== $data->getEntityType()) {
            $dataArray['entityType'] = $data->getEntityType();
        }
        if ($data->isInitialized('entityId') && null !== $data->getEntityId()) {
            $dataArray['entityId'] = $data->getEntityId();
        }
        if ($data->isInitialized('operation') && null !== $data->getOperation()) {
            $dataArray['operation'] = $data->getOperation();
        }
        if ($data->isInitialized('operationJson') && null !== $data->getOperationJson()) {
            $dataArray['operationJson'] = $data->getOperationJson();
        }
        if ($data->isInitialized('principalId') && null !== $data->getPrincipalId()) {
            $dataArray['principalId'] = $data->getPrincipalId();
        }
        if ($data->isInitialized('principalName') && null !== $data->getPrincipalName()) {
            $dataArray['principalName'] = $data->getPrincipalName();
        }
        if ($data->isInitialized('performedAt') && null !== $data->getPerformedAt()) {
            $dataArray['performedAt'] = $data->getPerformedAt()->format('Y-m-d\TH:i:sP');
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
        return [\FlowCatalyst\Generated\Model\AuditLogDetailDto::class => false];
    }
}