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
class PortalUserListItemNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PortalUserListItem::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PortalUserListItem::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PortalUserListItem();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('hasPassword', $data) && \is_int($data['hasPassword'])) {
            $data['hasPassword'] = (bool) $data['hasPassword'];
        }
        if (\array_key_exists('apps', $data) && $data['apps'] !== null) {
            $values = [];
            foreach ($data['apps'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\PortalUserAppRef::class, 'json', $context);
            }
            $object->setApps($values);
        }
        elseif (\array_key_exists('apps', $data) && $data['apps'] === null) {
            $object->setApps(null);
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
        if (\array_key_exists('hasPassword', $data) && $data['hasPassword'] !== null) {
            $object->setHasPassword($data['hasPassword']);
        }
        elseif (\array_key_exists('hasPassword', $data) && $data['hasPassword'] === null) {
            $object->setHasPassword(null);
        }
        if (\array_key_exists('identityId', $data) && $data['identityId'] !== null) {
            $object->setIdentityId($data['identityId']);
        }
        elseif (\array_key_exists('identityId', $data) && $data['identityId'] === null) {
            $object->setIdentityId(null);
        }
        if (\array_key_exists('inviteExpiresAt', $data) && $data['inviteExpiresAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['inviteExpiresAt']);
            if (false === $date_1) {
                if (is_string($data['inviteExpiresAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['inviteExpiresAt'])) {
                    try {
                        $date_1 = new \DateTime($data['inviteExpiresAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['inviteExpiresAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['inviteExpiresAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setInviteExpiresAt($date_1);
        }
        elseif (\array_key_exists('inviteExpiresAt', $data) && $data['inviteExpiresAt'] === null) {
            $object->setInviteExpiresAt(null);
        }
        if (\array_key_exists('invitedAt', $data) && $data['invitedAt'] !== null) {
            $date_2 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['invitedAt']);
            if (false === $date_2) {
                if (is_string($data['invitedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['invitedAt'])) {
                    try {
                        $date_2 = new \DateTime($data['invitedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['invitedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['invitedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setInvitedAt($date_2);
        }
        elseif (\array_key_exists('invitedAt', $data) && $data['invitedAt'] === null) {
            $object->setInvitedAt(null);
        }
        if (\array_key_exists('lastLoginAt', $data) && $data['lastLoginAt'] !== null) {
            $date_3 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['lastLoginAt']);
            if (false === $date_3) {
                if (is_string($data['lastLoginAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['lastLoginAt'])) {
                    try {
                        $date_3 = new \DateTime($data['lastLoginAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lastLoginAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['lastLoginAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setLastLoginAt($date_3);
        }
        elseif (\array_key_exists('lastLoginAt', $data) && $data['lastLoginAt'] === null) {
            $object->setLastLoginAt(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('state', $data) && $data['state'] !== null) {
            $object->setState($data['state']);
        }
        elseif (\array_key_exists('state', $data) && $data['state'] === null) {
            $object->setState(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $date_4 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']);
            if (false === $date_4) {
                if (is_string($data['updatedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['updatedAt'])) {
                    try {
                        $date_4 = new \DateTime($data['updatedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setUpdatedAt($date_4);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
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
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['email'] = $data->getEmail();
        $dataArray['hasPassword'] = $data->getHasPassword();
        $dataArray['identityId'] = $data->getIdentityId();
        if ($data->isInitialized('inviteExpiresAt') && null !== $data->getInviteExpiresAt()) {
            $dataArray['inviteExpiresAt'] = $data->getInviteExpiresAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('invitedAt') && null !== $data->getInvitedAt()) {
            $dataArray['invitedAt'] = $data->getInvitedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('lastLoginAt') && null !== $data->getLastLoginAt()) {
            $dataArray['lastLoginAt'] = $data->getLastLoginAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['name'] = $data->getName();
        $dataArray['source'] = $data->getSource();
        $dataArray['state'] = $data->getState();
        $dataArray['status'] = $data->getStatus();
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PortalUserListItem::class => false];
    }
}