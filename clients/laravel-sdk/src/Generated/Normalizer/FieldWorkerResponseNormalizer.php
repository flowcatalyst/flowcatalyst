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
class FieldWorkerResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FieldWorkerResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FieldWorkerResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FieldWorkerResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('canImpersonate', $data) && \is_int($data['canImpersonate'])) {
            $data['canImpersonate'] = (bool) $data['canImpersonate'];
        }
        if (\array_key_exists('created', $data) && \is_int($data['created'])) {
            $data['created'] = (bool) $data['created'];
        }
        if (\array_key_exists('hardLocked', $data) && \is_int($data['hardLocked'])) {
            $data['hardLocked'] = (bool) $data['hardLocked'];
        }
        if (\array_key_exists('hasPin', $data) && \is_int($data['hasPin'])) {
            $data['hasPin'] = (bool) $data['hasPin'];
        }
        if (\array_key_exists('isTest', $data) && \is_int($data['isTest'])) {
            $data['isTest'] = (bool) $data['isTest'];
        }
        if (\array_key_exists('locked', $data) && \is_int($data['locked'])) {
            $data['locked'] = (bool) $data['locked'];
        }
        if (\array_key_exists('pinChangeRequired', $data) && \is_int($data['pinChangeRequired'])) {
            $data['pinChangeRequired'] = (bool) $data['pinChangeRequired'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('apps', $data) && $data['apps'] !== null) {
            $values = [];
            foreach ($data['apps'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\FieldWorkerAppRef::class, 'json', $context);
            }
            $object->setApps($values);
        }
        elseif (\array_key_exists('apps', $data) && $data['apps'] === null) {
            $object->setApps(null);
        }
        if (\array_key_exists('badge', $data) && $data['badge'] !== null) {
            $object->setBadge($this->denormalizer->denormalize($data['badge'], \FlowCatalyst\Generated\Model\FieldWorkerBadgeRef::class, 'json', $context));
        }
        elseif (\array_key_exists('badge', $data) && $data['badge'] === null) {
            $object->setBadge(null);
        }
        if (\array_key_exists('canImpersonate', $data) && $data['canImpersonate'] !== null) {
            $object->setCanImpersonate($data['canImpersonate']);
        }
        elseif (\array_key_exists('canImpersonate', $data) && $data['canImpersonate'] === null) {
            $object->setCanImpersonate(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('created', $data) && $data['created'] !== null) {
            $object->setCreated($data['created']);
        }
        elseif (\array_key_exists('created', $data) && $data['created'] === null) {
            $object->setCreated(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']);
            if (false === $date) {
                if (is_string($data['createdAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['createdAt'])) {
                    try {
                        $date = new \DateTime($data['createdAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['createdAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['createdAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setCreatedAt($date);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('email', $data) && $data['email'] !== null) {
            $object->setEmail($data['email']);
        }
        elseif (\array_key_exists('email', $data) && $data['email'] === null) {
            $object->setEmail(null);
        }
        if (\array_key_exists('failedSinceSuccess', $data) && $data['failedSinceSuccess'] !== null) {
            $object->setFailedSinceSuccess($data['failedSinceSuccess']);
        }
        elseif (\array_key_exists('failedSinceSuccess', $data) && $data['failedSinceSuccess'] === null) {
            $object->setFailedSinceSuccess(null);
        }
        if (\array_key_exists('hardLocked', $data) && $data['hardLocked'] !== null) {
            $object->setHardLocked($data['hardLocked']);
        }
        elseif (\array_key_exists('hardLocked', $data) && $data['hardLocked'] === null) {
            $object->setHardLocked(null);
        }
        if (\array_key_exists('hasPin', $data) && $data['hasPin'] !== null) {
            $object->setHasPin($data['hasPin']);
        }
        elseif (\array_key_exists('hasPin', $data) && $data['hasPin'] === null) {
            $object->setHasPin(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('isTest', $data) && $data['isTest'] !== null) {
            $object->setIsTest($data['isTest']);
        }
        elseif (\array_key_exists('isTest', $data) && $data['isTest'] === null) {
            $object->setIsTest(null);
        }
        if (\array_key_exists('lastLoginAt', $data) && $data['lastLoginAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['lastLoginAt']);
            if (false === $date_1) {
                if (is_string($data['lastLoginAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['lastLoginAt'])) {
                    try {
                        $date_1 = new \DateTime($data['lastLoginAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lastLoginAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lastLoginAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setLastLoginAt($date_1);
        }
        elseif (\array_key_exists('lastLoginAt', $data) && $data['lastLoginAt'] === null) {
            $object->setLastLoginAt(null);
        }
        if (\array_key_exists('locked', $data) && $data['locked'] !== null) {
            $object->setLocked($data['locked']);
        }
        elseif (\array_key_exists('locked', $data) && $data['locked'] === null) {
            $object->setLocked(null);
        }
        if (\array_key_exists('lockedUntil', $data) && $data['lockedUntil'] !== null) {
            $date_2 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['lockedUntil']);
            if (false === $date_2) {
                if (is_string($data['lockedUntil']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['lockedUntil'])) {
                    try {
                        $date_2 = new \DateTime($data['lockedUntil']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lockedUntil'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lockedUntil'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setLockedUntil($date_2);
        }
        elseif (\array_key_exists('lockedUntil', $data) && $data['lockedUntil'] === null) {
            $object->setLockedUntil(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('pinChangeRequired', $data) && $data['pinChangeRequired'] !== null) {
            $object->setPinChangeRequired($data['pinChangeRequired']);
        }
        elseif (\array_key_exists('pinChangeRequired', $data) && $data['pinChangeRequired'] === null) {
            $object->setPinChangeRequired(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $date_3 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']);
            if (false === $date_3) {
                if (is_string($data['updatedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['updatedAt'])) {
                    try {
                        $date_3 = new \DateTime($data['updatedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setUpdatedAt($date_3);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        if (\array_key_exists('workerCode', $data) && $data['workerCode'] !== null) {
            $object->setWorkerCode($data['workerCode']);
        }
        elseif (\array_key_exists('workerCode', $data) && $data['workerCode'] === null) {
            $object->setWorkerCode(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getApps() as $value) {
            $values[] = $value === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value, 'json', $context));
        }
        $dataArray['apps'] = $values;
        $dataArray['badge'] = $data->getBadge() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getBadge(), 'json', $context));
        $dataArray['canImpersonate'] = $data->getCanImpersonate();
        $dataArray['clientId'] = $data->getClientId();
        if ($data->isInitialized('created') && null !== $data->getCreated()) {
            $dataArray['created'] = $data->getCreated();
        }
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('email') && null !== $data->getEmail()) {
            $dataArray['email'] = $data->getEmail();
        }
        $dataArray['failedSinceSuccess'] = $data->getFailedSinceSuccess();
        $dataArray['hardLocked'] = $data->getHardLocked();
        $dataArray['hasPin'] = $data->getHasPin();
        $dataArray['id'] = $data->getId();
        $dataArray['isTest'] = $data->getIsTest();
        if ($data->isInitialized('lastLoginAt') && null !== $data->getLastLoginAt()) {
            $dataArray['lastLoginAt'] = $data->getLastLoginAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['locked'] = $data->getLocked();
        if ($data->isInitialized('lockedUntil') && null !== $data->getLockedUntil()) {
            $dataArray['lockedUntil'] = $data->getLockedUntil()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['name'] = $data->getName();
        $dataArray['pinChangeRequired'] = $data->getPinChangeRequired();
        $dataArray['status'] = $data->getStatus();
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['workerCode'] = $data->getWorkerCode();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FieldWorkerResponse::class => false];
    }
}