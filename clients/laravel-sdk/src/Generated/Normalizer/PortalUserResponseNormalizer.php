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
class PortalUserResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PortalUserResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PortalUserResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PortalUserResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('created', $data) && \is_int($data['created'])) {
            $data['created'] = (bool) $data['created'];
        }
        if (\array_key_exists('invited', $data) && \is_int($data['invited'])) {
            $data['invited'] = (bool) $data['invited'];
        }
        if (\array_key_exists('ssoManaged', $data) && \is_int($data['ssoManaged'])) {
            $data['ssoManaged'] = (bool) $data['ssoManaged'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('created', $data) && $data['created'] !== null) {
            $object->setCreated($data['created']);
        }
        elseif (\array_key_exists('created', $data) && $data['created'] === null) {
            $object->setCreated(null);
        }
        if (\array_key_exists('identityId', $data) && $data['identityId'] !== null) {
            $object->setIdentityId($data['identityId']);
        }
        elseif (\array_key_exists('identityId', $data) && $data['identityId'] === null) {
            $object->setIdentityId(null);
        }
        if (\array_key_exists('inviteUrl', $data) && $data['inviteUrl'] !== null) {
            $object->setInviteUrl($data['inviteUrl']);
        }
        elseif (\array_key_exists('inviteUrl', $data) && $data['inviteUrl'] === null) {
            $object->setInviteUrl(null);
        }
        if (\array_key_exists('invited', $data) && $data['invited'] !== null) {
            $object->setInvited($data['invited']);
        }
        elseif (\array_key_exists('invited', $data) && $data['invited'] === null) {
            $object->setInvited(null);
        }
        if (\array_key_exists('ssoManaged', $data) && $data['ssoManaged'] !== null) {
            $object->setSsoManaged($data['ssoManaged']);
        }
        elseif (\array_key_exists('ssoManaged', $data) && $data['ssoManaged'] === null) {
            $object->setSsoManaged(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['created'] = $data->getCreated();
        $dataArray['identityId'] = $data->getIdentityId();
        if ($data->isInitialized('inviteUrl') && null !== $data->getInviteUrl()) {
            $dataArray['inviteUrl'] = $data->getInviteUrl();
        }
        $dataArray['invited'] = $data->getInvited();
        if ($data->isInitialized('ssoManaged') && null !== $data->getSsoManaged()) {
            $dataArray['ssoManaged'] = $data->getSsoManaged();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PortalUserResponse::class => false];
    }
}