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
        if (\array_key_exists('badgeOnlyLogin', $data) && \is_int($data['badgeOnlyLogin'])) {
            $data['badgeOnlyLogin'] = (bool) $data['badgeOnlyLogin'];
        }
        if (\array_key_exists('requireAttestation', $data) && \is_int($data['requireAttestation'])) {
            $data['requireAttestation'] = (bool) $data['requireAttestation'];
        }
        if (\array_key_exists('requireEnrolledDevice', $data) && \is_int($data['requireEnrolledDevice'])) {
            $data['requireEnrolledDevice'] = (bool) $data['requireEnrolledDevice'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
            unset($data['$schema']);
        }
        if (\array_key_exists('badgeOnlyLogin', $data) && $data['badgeOnlyLogin'] !== null) {
            $object->setBadgeOnlyLogin($data['badgeOnlyLogin']);
            unset($data['badgeOnlyLogin']);
        }
        elseif (\array_key_exists('badgeOnlyLogin', $data) && $data['badgeOnlyLogin'] === null) {
            $object->setBadgeOnlyLogin(null);
            unset($data['badgeOnlyLogin']);
        }
        if (\array_key_exists('hardLockThreshold', $data) && $data['hardLockThreshold'] !== null) {
            $object->setHardLockThreshold($data['hardLockThreshold']);
            unset($data['hardLockThreshold']);
        }
        elseif (\array_key_exists('hardLockThreshold', $data) && $data['hardLockThreshold'] === null) {
            $object->setHardLockThreshold(null);
            unset($data['hardLockThreshold']);
        }
        if (\array_key_exists('impersonationMaxSecs', $data) && $data['impersonationMaxSecs'] !== null) {
            $object->setImpersonationMaxSecs($data['impersonationMaxSecs']);
            unset($data['impersonationMaxSecs']);
        }
        elseif (\array_key_exists('impersonationMaxSecs', $data) && $data['impersonationMaxSecs'] === null) {
            $object->setImpersonationMaxSecs(null);
            unset($data['impersonationMaxSecs']);
        }
        if (\array_key_exists('personalIdleSecs', $data) && $data['personalIdleSecs'] !== null) {
            $object->setPersonalIdleSecs($data['personalIdleSecs']);
            unset($data['personalIdleSecs']);
        }
        elseif (\array_key_exists('personalIdleSecs', $data) && $data['personalIdleSecs'] === null) {
            $object->setPersonalIdleSecs(null);
            unset($data['personalIdleSecs']);
        }
        if (\array_key_exists('pinMinLength', $data) && $data['pinMinLength'] !== null) {
            $object->setPinMinLength($data['pinMinLength']);
            unset($data['pinMinLength']);
        }
        elseif (\array_key_exists('pinMinLength', $data) && $data['pinMinLength'] === null) {
            $object->setPinMinLength(null);
            unset($data['pinMinLength']);
        }
        if (\array_key_exists('pinReauthDays', $data) && $data['pinReauthDays'] !== null) {
            $object->setPinReauthDays($data['pinReauthDays']);
            unset($data['pinReauthDays']);
        }
        elseif (\array_key_exists('pinReauthDays', $data) && $data['pinReauthDays'] === null) {
            $object->setPinReauthDays(null);
            unset($data['pinReauthDays']);
        }
        if (\array_key_exists('requireAttestation', $data) && $data['requireAttestation'] !== null) {
            $object->setRequireAttestation($data['requireAttestation']);
            unset($data['requireAttestation']);
        }
        elseif (\array_key_exists('requireAttestation', $data) && $data['requireAttestation'] === null) {
            $object->setRequireAttestation(null);
            unset($data['requireAttestation']);
        }
        if (\array_key_exists('requireEnrolledDevice', $data) && $data['requireEnrolledDevice'] !== null) {
            $object->setRequireEnrolledDevice($data['requireEnrolledDevice']);
            unset($data['requireEnrolledDevice']);
        }
        elseif (\array_key_exists('requireEnrolledDevice', $data) && $data['requireEnrolledDevice'] === null) {
            $object->setRequireEnrolledDevice(null);
            unset($data['requireEnrolledDevice']);
        }
        if (\array_key_exists('sharedIdleSecs', $data) && $data['sharedIdleSecs'] !== null) {
            $object->setSharedIdleSecs($data['sharedIdleSecs']);
            unset($data['sharedIdleSecs']);
        }
        elseif (\array_key_exists('sharedIdleSecs', $data) && $data['sharedIdleSecs'] === null) {
            $object->setSharedIdleSecs(null);
            unset($data['sharedIdleSecs']);
        }
        if (\array_key_exists('shiftMaxSecs', $data) && $data['shiftMaxSecs'] !== null) {
            $object->setShiftMaxSecs($data['shiftMaxSecs']);
            unset($data['shiftMaxSecs']);
        }
        elseif (\array_key_exists('shiftMaxSecs', $data) && $data['shiftMaxSecs'] === null) {
            $object->setShiftMaxSecs(null);
            unset($data['shiftMaxSecs']);
        }
        if (\array_key_exists('softLockSecs', $data) && $data['softLockSecs'] !== null) {
            $object->setSoftLockSecs($data['softLockSecs']);
            unset($data['softLockSecs']);
        }
        elseif (\array_key_exists('softLockSecs', $data) && $data['softLockSecs'] === null) {
            $object->setSoftLockSecs(null);
            unset($data['softLockSecs']);
        }
        if (\array_key_exists('softLockThreshold', $data) && $data['softLockThreshold'] !== null) {
            $object->setSoftLockThreshold($data['softLockThreshold']);
            unset($data['softLockThreshold']);
        }
        elseif (\array_key_exists('softLockThreshold', $data) && $data['softLockThreshold'] === null) {
            $object->setSoftLockThreshold(null);
            unset($data['softLockThreshold']);
        }
        if (\array_key_exists('unenrolledIdleSecs', $data) && $data['unenrolledIdleSecs'] !== null) {
            $object->setUnenrolledIdleSecs($data['unenrolledIdleSecs']);
            unset($data['unenrolledIdleSecs']);
        }
        elseif (\array_key_exists('unenrolledIdleSecs', $data) && $data['unenrolledIdleSecs'] === null) {
            $object->setUnenrolledIdleSecs(null);
            unset($data['unenrolledIdleSecs']);
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
        if ($data->isInitialized('badgeOnlyLogin') && null !== $data->getBadgeOnlyLogin()) {
            $dataArray['badgeOnlyLogin'] = $data->getBadgeOnlyLogin();
        }
        if ($data->isInitialized('hardLockThreshold') && null !== $data->getHardLockThreshold()) {
            $dataArray['hardLockThreshold'] = $data->getHardLockThreshold();
        }
        if ($data->isInitialized('impersonationMaxSecs') && null !== $data->getImpersonationMaxSecs()) {
            $dataArray['impersonationMaxSecs'] = $data->getImpersonationMaxSecs();
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
        if ($data->isInitialized('requireEnrolledDevice') && null !== $data->getRequireEnrolledDevice()) {
            $dataArray['requireEnrolledDevice'] = $data->getRequireEnrolledDevice();
        }
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
        if ($data->isInitialized('unenrolledIdleSecs') && null !== $data->getUnenrolledIdleSecs()) {
            $dataArray['unenrolledIdleSecs'] = $data->getUnenrolledIdleSecs();
        }
        foreach ($data->additionalPropertyEntries() as $key => $value) {
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