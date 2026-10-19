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
class ResendInvitationRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ResendInvitationRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ResendInvitationRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\ResendInvitationRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('returnInviteLink', $data) && \is_int($data['returnInviteLink'])) {
            $data['returnInviteLink'] = (bool) $data['returnInviteLink'];
        }
        if (\array_key_exists('sendInvitation', $data) && \is_int($data['sendInvitation'])) {
            $data['sendInvitation'] = (bool) $data['sendInvitation'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('inviteRedirectUri', $data) && $data['inviteRedirectUri'] !== null) {
            $object->setInviteRedirectUri($data['inviteRedirectUri']);
            unset($data['inviteRedirectUri']);
        }
        elseif (\array_key_exists('inviteRedirectUri', $data) && $data['inviteRedirectUri'] === null) {
            $object->setInviteRedirectUri(null);
        }
        if (\array_key_exists('returnInviteLink', $data) && $data['returnInviteLink'] !== null) {
            $object->setReturnInviteLink($data['returnInviteLink']);
            unset($data['returnInviteLink']);
        }
        elseif (\array_key_exists('returnInviteLink', $data) && $data['returnInviteLink'] === null) {
            $object->setReturnInviteLink(null);
        }
        if (\array_key_exists('sendInvitation', $data) && $data['sendInvitation'] !== null) {
            $object->setSendInvitation($data['sendInvitation']);
            unset($data['sendInvitation']);
        }
        elseif (\array_key_exists('sendInvitation', $data) && $data['sendInvitation'] === null) {
            $object->setSendInvitation(null);
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
        if ($data->isInitialized('inviteRedirectUri') && null !== $data->getInviteRedirectUri()) {
            $dataArray['inviteRedirectUri'] = $data->getInviteRedirectUri();
        }
        if ($data->isInitialized('returnInviteLink') && null !== $data->getReturnInviteLink()) {
            $dataArray['returnInviteLink'] = $data->getReturnInviteLink();
        }
        if ($data->isInitialized('sendInvitation') && null !== $data->getSendInvitation()) {
            $dataArray['sendInvitation'] = $data->getSendInvitation();
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
        return [\FlowCatalyst\Generated\Model\ResendInvitationRequest::class => false];
    }
}