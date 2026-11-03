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
class PutFieldPolicyRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PutFieldPolicyRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PutFieldPolicyRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PutFieldPolicyRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('requireAttestation', $data) && \is_int($data['requireAttestation'])) {
            $data['requireAttestation'] = (bool) $data['requireAttestation'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('hardLockThreshold', $data) && $data['hardLockThreshold'] !== null) {
            $object->setHardLockThreshold($data['hardLockThreshold']);
            unset($data['hardLockThreshold']);
        }
        elseif (\array_key_exists('hardLockThreshold', $data) && $data['hardLockThreshold'] === null) {
            $object->setHardLockThreshold(null);
        }
        if (\array_key_exists('personalIdleSecs', $data) && $data['personalIdleSecs'] !== null) {
            $object->setPersonalIdleSecs($data['personalIdleSecs']);
            unset($data['personalIdleSecs']);
        }
        elseif (\array_key_exists('personalIdleSecs', $data) && $data['personalIdleSecs'] === null) {
            $object->setPersonalIdleSecs(null);
        }
        if (\array_key_exists('pinMinLength', $data) && $data['pinMinLength'] !== null) {
            $object->setPinMinLength($data['pinMinLength']);
            unset($data['pinMinLength']);
        }
        elseif (\array_key_exists('pinMinLength', $data) && $data['pinMinLength'] === null) {
            $object->setPinMinLength(null);
        }
        if (\array_key_exists('pinReauthDays', $data) && $data['pinReauthDays'] !== null) {
            $object->setPinReauthDays($data['pinReauthDays']);
            unset($data['pinReauthDays']);
        }
        elseif (\array_key_exists('pinReauthDays', $data) && $data['pinReauthDays'] === null) {
            $object->setPinReauthDays(null);
        }
        if (\array_key_exists('requireAttestation', $data) && $data['requireAttestation'] !== null) {
            $object->setRequireAttestation($data['requireAttestation']);
            unset($data['requireAttestation']);
        }
        elseif (\array_key_exists('requireAttestation', $data) && $data['requireAttestation'] === null) {
            $object->setRequireAttestation(null);
        }
        if (\array_key_exists('sharedIdleSecs', $data) && $data['sharedIdleSecs'] !== null) {
            $object->setSharedIdleSecs($data['sharedIdleSecs']);
            unset($data['sharedIdleSecs']);
        }
        elseif (\array_key_exists('sharedIdleSecs', $data) && $data['sharedIdleSecs'] === null) {
            $object->setSharedIdleSecs(null);
        }
        if (\array_key_exists('shiftMaxSecs', $data) && $data['shiftMaxSecs'] !== null) {
            $object->setShiftMaxSecs($data['shiftMaxSecs']);
            unset($data['shiftMaxSecs']);
        }
        elseif (\array_key_exists('shiftMaxSecs', $data) && $data['shiftMaxSecs'] === null) {
            $object->setShiftMaxSecs(null);
        }
        if (\array_key_exists('softLockSecs', $data) && $data['softLockSecs'] !== null) {
            $object->setSoftLockSecs($data['softLockSecs']);
            unset($data['softLockSecs']);
        }
        elseif (\array_key_exists('softLockSecs', $data) && $data['softLockSecs'] === null) {
            $object->setSoftLockSecs(null);
        }
        if (\array_key_exists('softLockThreshold', $data) && $data['softLockThreshold'] !== null) {
            $object->setSoftLockThreshold($data['softLockThreshold']);
            unset($data['softLockThreshold']);
        }
        elseif (\array_key_exists('softLockThreshold', $data) && $data['softLockThreshold'] === null) {
            $object->setSoftLockThreshold(null);
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
        if ($data->isInitialized('hardLockThreshold') && null !== $data->getHardLockThreshold()) {
            $dataArray['hardLockThreshold'] = $data->getHardLockThreshold();
        }
        if ($data->isInitialized('personalIdleSecs') && null !== $data->getPersonalIdleSecs()) {
            $dataArray['personalIdleSecs'] = $data->getPersonalIdleSecs();
        }
        if ($data->isInitialized('pinMinLength') && null !== $data->getPinMinLength()) {
            $dataArray['pinMinLength'] = $data->getPinMinLength();
        }
        if ($data->isInitialized('pinReauthDays') && null !== $data->getPinReauthDays()) {
            $dataArray['pinReauthDays'] = $data->getPinReauthDays();
        }
        $dataArray['requireAttestation'] = $data->getRequireAttestation();
        if ($data->isInitialized('sharedIdleSecs') && null !== $data->getSharedIdleSecs()) {
            $dataArray['sharedIdleSecs'] = $data->getSharedIdleSecs();
        }
        if ($data->isInitialized('shiftMaxSecs') && null !== $data->getShiftMaxSecs()) {
            $dataArray['shiftMaxSecs'] = $data->getShiftMaxSecs();
        }
        if ($data->isInitialized('softLockSecs') && null !== $data->getSoftLockSecs()) {
            $dataArray['softLockSecs'] = $data->getSoftLockSecs();
        }
        if ($data->isInitialized('softLockThreshold') && null !== $data->getSoftLockThreshold()) {
            $dataArray['softLockThreshold'] = $data->getSoftLockThreshold();
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
        return [\FlowCatalyst\Generated\Model\PutFieldPolicyRequest::class => false];
    }
}