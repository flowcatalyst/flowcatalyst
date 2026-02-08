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
class EmailDomainMappingDtoNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\EmailDomainMappingDto::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\EmailDomainMappingDto::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\EmailDomainMappingDto();
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
        if (\array_key_exists('emailDomain', $data) && $data['emailDomain'] !== null) {
            $object->setEmailDomain($data['emailDomain']);
            unset($data['emailDomain']);
        }
        elseif (\array_key_exists('emailDomain', $data) && $data['emailDomain'] === null) {
            $object->setEmailDomain(null);
        }
        if (\array_key_exists('identityProviderId', $data) && $data['identityProviderId'] !== null) {
            $object->setIdentityProviderId($data['identityProviderId']);
            unset($data['identityProviderId']);
        }
        elseif (\array_key_exists('identityProviderId', $data) && $data['identityProviderId'] === null) {
            $object->setIdentityProviderId(null);
        }
        if (\array_key_exists('identityProviderCode', $data) && $data['identityProviderCode'] !== null) {
            $object->setIdentityProviderCode($data['identityProviderCode']);
            unset($data['identityProviderCode']);
        }
        elseif (\array_key_exists('identityProviderCode', $data) && $data['identityProviderCode'] === null) {
            $object->setIdentityProviderCode(null);
        }
        if (\array_key_exists('identityProviderName', $data) && $data['identityProviderName'] !== null) {
            $object->setIdentityProviderName($data['identityProviderName']);
            unset($data['identityProviderName']);
        }
        elseif (\array_key_exists('identityProviderName', $data) && $data['identityProviderName'] === null) {
            $object->setIdentityProviderName(null);
        }
        if (\array_key_exists('scopeType', $data) && $data['scopeType'] !== null) {
            $object->setScopeType($data['scopeType']);
            unset($data['scopeType']);
        }
        elseif (\array_key_exists('scopeType', $data) && $data['scopeType'] === null) {
            $object->setScopeType(null);
        }
        if (\array_key_exists('primaryClientId', $data) && $data['primaryClientId'] !== null) {
            $object->setPrimaryClientId($data['primaryClientId']);
            unset($data['primaryClientId']);
        }
        elseif (\array_key_exists('primaryClientId', $data) && $data['primaryClientId'] === null) {
            $object->setPrimaryClientId(null);
        }
        if (\array_key_exists('additionalClientIds', $data) && $data['additionalClientIds'] !== null) {
            $values = [];
            foreach ($data['additionalClientIds'] as $value) {
                $values[] = $value;
            }
            $object->setAdditionalClientIds($values);
            unset($data['additionalClientIds']);
        }
        elseif (\array_key_exists('additionalClientIds', $data) && $data['additionalClientIds'] === null) {
            $object->setAdditionalClientIds(null);
        }
        if (\array_key_exists('grantedClientIds', $data) && $data['grantedClientIds'] !== null) {
            $values_1 = [];
            foreach ($data['grantedClientIds'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setGrantedClientIds($values_1);
            unset($data['grantedClientIds']);
        }
        elseif (\array_key_exists('grantedClientIds', $data) && $data['grantedClientIds'] === null) {
            $object->setGrantedClientIds(null);
        }
        if (\array_key_exists('allAccessibleClientIds', $data) && $data['allAccessibleClientIds'] !== null) {
            $values_2 = [];
            foreach ($data['allAccessibleClientIds'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setAllAccessibleClientIds($values_2);
            unset($data['allAccessibleClientIds']);
        }
        elseif (\array_key_exists('allAccessibleClientIds', $data) && $data['allAccessibleClientIds'] === null) {
            $object->setAllAccessibleClientIds(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
            unset($data['updatedAt']);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        foreach ($data as $key => $value_3) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_3;
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
        if ($data->isInitialized('emailDomain') && null !== $data->getEmailDomain()) {
            $dataArray['emailDomain'] = $data->getEmailDomain();
        }
        if ($data->isInitialized('identityProviderId') && null !== $data->getIdentityProviderId()) {
            $dataArray['identityProviderId'] = $data->getIdentityProviderId();
        }
        if ($data->isInitialized('identityProviderCode') && null !== $data->getIdentityProviderCode()) {
            $dataArray['identityProviderCode'] = $data->getIdentityProviderCode();
        }
        if ($data->isInitialized('identityProviderName') && null !== $data->getIdentityProviderName()) {
            $dataArray['identityProviderName'] = $data->getIdentityProviderName();
        }
        if ($data->isInitialized('scopeType') && null !== $data->getScopeType()) {
            $dataArray['scopeType'] = $data->getScopeType();
        }
        if ($data->isInitialized('primaryClientId') && null !== $data->getPrimaryClientId()) {
            $dataArray['primaryClientId'] = $data->getPrimaryClientId();
        }
        if ($data->isInitialized('additionalClientIds') && null !== $data->getAdditionalClientIds()) {
            $values = [];
            foreach ($data->getAdditionalClientIds() as $value) {
                $values[] = $value;
            }
            $dataArray['additionalClientIds'] = $values;
        }
        if ($data->isInitialized('grantedClientIds') && null !== $data->getGrantedClientIds()) {
            $values_1 = [];
            foreach ($data->getGrantedClientIds() as $value_1) {
                $values_1[] = $value_1;
            }
            $dataArray['grantedClientIds'] = $values_1;
        }
        if ($data->isInitialized('allAccessibleClientIds') && null !== $data->getAllAccessibleClientIds()) {
            $values_2 = [];
            foreach ($data->getAllAccessibleClientIds() as $value_2) {
                $values_2[] = $value_2;
            }
            $dataArray['allAccessibleClientIds'] = $values_2;
        }
        if ($data->isInitialized('createdAt') && null !== $data->getCreatedAt()) {
            $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('updatedAt') && null !== $data->getUpdatedAt()) {
            $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        }
        foreach ($data as $key => $value_3) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_3;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\EmailDomainMappingDto::class => false];
    }
}