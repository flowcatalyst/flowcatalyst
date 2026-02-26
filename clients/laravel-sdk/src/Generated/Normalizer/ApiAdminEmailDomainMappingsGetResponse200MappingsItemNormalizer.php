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
class ApiAdminEmailDomainMappingsGetResponse200MappingsItemNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200MappingsItem::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200MappingsItem::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200MappingsItem();
        if (\array_key_exists('syncRolesFromIdp', $data) && \is_int($data['syncRolesFromIdp'])) {
            $data['syncRolesFromIdp'] = (bool) $data['syncRolesFromIdp'];
        }
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
        if (\array_key_exists('identityProviderName', $data) && $data['identityProviderName'] !== null) {
            $object->setIdentityProviderName($data['identityProviderName']);
            unset($data['identityProviderName']);
        }
        elseif (\array_key_exists('identityProviderName', $data) && $data['identityProviderName'] === null) {
            $object->setIdentityProviderName(null);
        }
        if (\array_key_exists('identityProviderType', $data) && $data['identityProviderType'] !== null) {
            $object->setIdentityProviderType($data['identityProviderType']);
            unset($data['identityProviderType']);
        }
        elseif (\array_key_exists('identityProviderType', $data) && $data['identityProviderType'] === null) {
            $object->setIdentityProviderType(null);
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
        if (\array_key_exists('primaryClientName', $data) && $data['primaryClientName'] !== null) {
            $object->setPrimaryClientName($data['primaryClientName']);
            unset($data['primaryClientName']);
        }
        elseif (\array_key_exists('primaryClientName', $data) && $data['primaryClientName'] === null) {
            $object->setPrimaryClientName(null);
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
        if (\array_key_exists('requiredOidcTenantId', $data) && $data['requiredOidcTenantId'] !== null) {
            $object->setRequiredOidcTenantId($data['requiredOidcTenantId']);
            unset($data['requiredOidcTenantId']);
        }
        elseif (\array_key_exists('requiredOidcTenantId', $data) && $data['requiredOidcTenantId'] === null) {
            $object->setRequiredOidcTenantId(null);
        }
        if (\array_key_exists('allowedRoleIds', $data) && $data['allowedRoleIds'] !== null) {
            $values_2 = [];
            foreach ($data['allowedRoleIds'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setAllowedRoleIds($values_2);
            unset($data['allowedRoleIds']);
        }
        elseif (\array_key_exists('allowedRoleIds', $data) && $data['allowedRoleIds'] === null) {
            $object->setAllowedRoleIds(null);
        }
        if (\array_key_exists('syncRolesFromIdp', $data) && $data['syncRolesFromIdp'] !== null) {
            $object->setSyncRolesFromIdp($data['syncRolesFromIdp']);
            unset($data['syncRolesFromIdp']);
        }
        elseif (\array_key_exists('syncRolesFromIdp', $data) && $data['syncRolesFromIdp'] === null) {
            $object->setSyncRolesFromIdp(null);
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
        $dataArray['id'] = $data->getId();
        $dataArray['emailDomain'] = $data->getEmailDomain();
        $dataArray['identityProviderId'] = $data->getIdentityProviderId();
        $dataArray['identityProviderName'] = $data->getIdentityProviderName();
        $dataArray['identityProviderType'] = $data->getIdentityProviderType();
        $dataArray['scopeType'] = $data->getScopeType();
        $dataArray['primaryClientId'] = $data->getPrimaryClientId();
        $dataArray['primaryClientName'] = $data->getPrimaryClientName();
        $values = [];
        foreach ($data->getAdditionalClientIds() as $value) {
            $values[] = $value;
        }
        $dataArray['additionalClientIds'] = $values;
        $values_1 = [];
        foreach ($data->getGrantedClientIds() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['grantedClientIds'] = $values_1;
        $dataArray['requiredOidcTenantId'] = $data->getRequiredOidcTenantId();
        $values_2 = [];
        foreach ($data->getAllowedRoleIds() as $value_2) {
            $values_2[] = $value_2;
        }
        $dataArray['allowedRoleIds'] = $values_2;
        $dataArray['syncRolesFromIdp'] = $data->getSyncRolesFromIdp();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value_3) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_3;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200MappingsItem::class => false];
    }
}