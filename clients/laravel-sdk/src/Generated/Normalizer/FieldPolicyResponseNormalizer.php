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
class FieldPolicyResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FieldPolicyResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FieldPolicyResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FieldPolicyResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('isDefault', $data) && \is_int($data['isDefault'])) {
            $data['isDefault'] = (bool) $data['isDefault'];
        }
        if (\array_key_exists('requireAttestation', $data) && \is_int($data['requireAttestation'])) {
            $data['requireAttestation'] = (bool) $data['requireAttestation'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('hardLockThreshold', $data) && $data['hardLockThreshold'] !== null) {
            $object->setHardLockThreshold($data['hardLockThreshold']);
        }
        elseif (\array_key_exists('hardLockThreshold', $data) && $data['hardLockThreshold'] === null) {
            $object->setHardLockThreshold(null);
        }
        if (\array_key_exists('isDefault', $data) && $data['isDefault'] !== null) {
            $object->setIsDefault($data['isDefault']);
        }
        elseif (\array_key_exists('isDefault', $data) && $data['isDefault'] === null) {
            $object->setIsDefault(null);
        }
        if (\array_key_exists('personalIdleSecs', $data) && $data['personalIdleSecs'] !== null) {
            $object->setPersonalIdleSecs($data['personalIdleSecs']);
        }
        elseif (\array_key_exists('personalIdleSecs', $data) && $data['personalIdleSecs'] === null) {
            $object->setPersonalIdleSecs(null);
        }
        if (\array_key_exists('pinMinLength', $data) && $data['pinMinLength'] !== null) {
            $object->setPinMinLength($data['pinMinLength']);
        }
        elseif (\array_key_exists('pinMinLength', $data) && $data['pinMinLength'] === null) {
            $object->setPinMinLength(null);
        }
        if (\array_key_exists('pinReauthDays', $data) && $data['pinReauthDays'] !== null) {
            $object->setPinReauthDays($data['pinReauthDays']);
        }
        elseif (\array_key_exists('pinReauthDays', $data) && $data['pinReauthDays'] === null) {
            $object->setPinReauthDays(null);
        }
        if (\array_key_exists('requireAttestation', $data) && $data['requireAttestation'] !== null) {
            $object->setRequireAttestation($data['requireAttestation']);
        }
        elseif (\array_key_exists('requireAttestation', $data) && $data['requireAttestation'] === null) {
            $object->setRequireAttestation(null);
        }
        if (\array_key_exists('sharedIdleSecs', $data) && $data['sharedIdleSecs'] !== null) {
            $object->setSharedIdleSecs($data['sharedIdleSecs']);
        }
        elseif (\array_key_exists('sharedIdleSecs', $data) && $data['sharedIdleSecs'] === null) {
            $object->setSharedIdleSecs(null);
        }
        if (\array_key_exists('shiftMaxSecs', $data) && $data['shiftMaxSecs'] !== null) {
            $object->setShiftMaxSecs($data['shiftMaxSecs']);
        }
        elseif (\array_key_exists('shiftMaxSecs', $data) && $data['shiftMaxSecs'] === null) {
            $object->setShiftMaxSecs(null);
        }
        if (\array_key_exists('softLockSecs', $data) && $data['softLockSecs'] !== null) {
            $object->setSoftLockSecs($data['softLockSecs']);
        }
        elseif (\array_key_exists('softLockSecs', $data) && $data['softLockSecs'] === null) {
            $object->setSoftLockSecs(null);
        }
        if (\array_key_exists('softLockThreshold', $data) && $data['softLockThreshold'] !== null) {
            $object->setSoftLockThreshold($data['softLockThreshold']);
        }
        elseif (\array_key_exists('softLockThreshold', $data) && $data['softLockThreshold'] === null) {
            $object->setSoftLockThreshold(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['hardLockThreshold'] = $data->getHardLockThreshold();
        $dataArray['isDefault'] = $data->getIsDefault();
        $dataArray['personalIdleSecs'] = $data->getPersonalIdleSecs();
        $dataArray['pinMinLength'] = $data->getPinMinLength();
        $dataArray['pinReauthDays'] = $data->getPinReauthDays();
        $dataArray['requireAttestation'] = $data->getRequireAttestation();
        $dataArray['sharedIdleSecs'] = $data->getSharedIdleSecs();
        $dataArray['shiftMaxSecs'] = $data->getShiftMaxSecs();
        $dataArray['softLockSecs'] = $data->getSoftLockSecs();
        $dataArray['softLockThreshold'] = $data->getSoftLockThreshold();
        if ($data->isInitialized('updatedAt') && null !== $data->getUpdatedAt()) {
            $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FieldPolicyResponse::class => false];
    }
}