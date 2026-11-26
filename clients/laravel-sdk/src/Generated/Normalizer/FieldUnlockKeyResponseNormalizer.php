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
class FieldUnlockKeyResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FieldUnlockKeyResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FieldUnlockKeyResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FieldUnlockKeyResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('attestation', $data) && $data['attestation'] !== null) {
            $object->setAttestation($this->denormalizer->denormalize($data['attestation'], \FlowCatalyst\Generated\Model\FieldUnlockKeyAttestation::class, 'json', $context));
        }
        elseif (\array_key_exists('attestation', $data) && $data['attestation'] === null) {
            $object->setAttestation(null);
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
        if (\array_key_exists('deviceId', $data) && $data['deviceId'] !== null) {
            $object->setDeviceId($data['deviceId']);
        }
        elseif (\array_key_exists('deviceId', $data) && $data['deviceId'] === null) {
            $object->setDeviceId(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('lastUsedAt', $data) && $data['lastUsedAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['lastUsedAt']);
            if (false === $date_1) {
                if (is_string($data['lastUsedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['lastUsedAt'])) {
                    try {
                        $date_1 = new \DateTime($data['lastUsedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lastUsedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lastUsedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setLastUsedAt($date_1);
        }
        elseif (\array_key_exists('lastUsedAt', $data) && $data['lastUsedAt'] === null) {
            $object->setLastUsedAt(null);
        }
        if (\array_key_exists('liveSessions', $data) && $data['liveSessions'] !== null) {
            $object->setLiveSessions($data['liveSessions']);
        }
        elseif (\array_key_exists('liveSessions', $data) && $data['liveSessions'] === null) {
            $object->setLiveSessions(null);
        }
        if (\array_key_exists('pinRequiredBy', $data) && $data['pinRequiredBy'] !== null) {
            $date_2 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['pinRequiredBy']);
            if (false === $date_2) {
                if (is_string($data['pinRequiredBy']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['pinRequiredBy'])) {
                    try {
                        $date_2 = new \DateTime($data['pinRequiredBy']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['pinRequiredBy'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['pinRequiredBy'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setPinRequiredBy($date_2);
        }
        elseif (\array_key_exists('pinRequiredBy', $data) && $data['pinRequiredBy'] === null) {
            $object->setPinRequiredBy(null);
        }
        if (\array_key_exists('pinVerifiedAt', $data) && $data['pinVerifiedAt'] !== null) {
            $date_3 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['pinVerifiedAt']);
            if (false === $date_3) {
                if (is_string($data['pinVerifiedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['pinVerifiedAt'])) {
                    try {
                        $date_3 = new \DateTime($data['pinVerifiedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['pinVerifiedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['pinVerifiedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setPinVerifiedAt($date_3);
        }
        elseif (\array_key_exists('pinVerifiedAt', $data) && $data['pinVerifiedAt'] === null) {
            $object->setPinVerifiedAt(null);
        }
        if (\array_key_exists('revokedAt', $data) && $data['revokedAt'] !== null) {
            $date_4 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['revokedAt']);
            if (false === $date_4) {
                if (is_string($data['revokedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['revokedAt'])) {
                    try {
                        $date_4 = new \DateTime($data['revokedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['revokedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['revokedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setRevokedAt($date_4);
        }
        elseif (\array_key_exists('revokedAt', $data) && $data['revokedAt'] === null) {
            $object->setRevokedAt(null);
        }
        if (\array_key_exists('revokedBy', $data) && $data['revokedBy'] !== null) {
            $object->setRevokedBy($data['revokedBy']);
        }
        elseif (\array_key_exists('revokedBy', $data) && $data['revokedBy'] === null) {
            $object->setRevokedBy(null);
        }
        if (\array_key_exists('revokedReason', $data) && $data['revokedReason'] !== null) {
            $object->setRevokedReason($data['revokedReason']);
        }
        elseif (\array_key_exists('revokedReason', $data) && $data['revokedReason'] === null) {
            $object->setRevokedReason(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('workerId', $data) && $data['workerId'] !== null) {
            $object->setWorkerId($data['workerId']);
        }
        elseif (\array_key_exists('workerId', $data) && $data['workerId'] === null) {
            $object->setWorkerId(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['attestation'] = $data->getAttestation() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getAttestation(), 'json', $context));
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['deviceId'] = $data->getDeviceId();
        $dataArray['id'] = $data->getId();
        if ($data->isInitialized('lastUsedAt') && null !== $data->getLastUsedAt()) {
            $dataArray['lastUsedAt'] = $data->getLastUsedAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['liveSessions'] = $data->getLiveSessions();
        if ($data->isInitialized('pinRequiredBy') && null !== $data->getPinRequiredBy()) {
            $dataArray['pinRequiredBy'] = $data->getPinRequiredBy()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['pinVerifiedAt'] = $data->getPinVerifiedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('revokedAt') && null !== $data->getRevokedAt()) {
            $dataArray['revokedAt'] = $data->getRevokedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('revokedBy') && null !== $data->getRevokedBy()) {
            $dataArray['revokedBy'] = $data->getRevokedBy();
        }
        if ($data->isInitialized('revokedReason') && null !== $data->getRevokedReason()) {
            $dataArray['revokedReason'] = $data->getRevokedReason();
        }
        $dataArray['status'] = $data->getStatus();
        $dataArray['workerId'] = $data->getWorkerId();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FieldUnlockKeyResponse::class => false];
    }
}