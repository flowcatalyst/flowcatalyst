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
class OAuthClientResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\OAuthClientResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\OAuthClientResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\OAuthClientResponse();
        if (\array_key_exists('active', $data) && \is_int($data['active'])) {
            $data['active'] = (bool) $data['active'];
        }
        if (\array_key_exists('pkceRequired', $data) && \is_int($data['pkceRequired'])) {
            $data['pkceRequired'] = (bool) $data['pkceRequired'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('active', $data) && $data['active'] !== null) {
            $object->setActive($data['active']);
            unset($data['active']);
        }
        elseif (\array_key_exists('active', $data) && $data['active'] === null) {
            $object->setActive(null);
        }
        if (\array_key_exists('allowedOrigins', $data) && $data['allowedOrigins'] !== null) {
            $values = [];
            foreach ($data['allowedOrigins'] as $value) {
                $values[] = $value;
            }
            $object->setAllowedOrigins($values);
            unset($data['allowedOrigins']);
        }
        elseif (\array_key_exists('allowedOrigins', $data) && $data['allowedOrigins'] === null) {
            $object->setAllowedOrigins(null);
        }
        if (\array_key_exists('applicationIds', $data) && $data['applicationIds'] !== null) {
            $values_1 = [];
            foreach ($data['applicationIds'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setApplicationIds($values_1);
            unset($data['applicationIds']);
        }
        elseif (\array_key_exists('applicationIds', $data) && $data['applicationIds'] === null) {
            $object->setApplicationIds(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
            unset($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('clientName', $data) && $data['clientName'] !== null) {
            $object->setClientName($data['clientName']);
            unset($data['clientName']);
        }
        elseif (\array_key_exists('clientName', $data) && $data['clientName'] === null) {
            $object->setClientName(null);
        }
        if (\array_key_exists('clientType', $data) && $data['clientType'] !== null) {
            $object->setClientType($data['clientType']);
            unset($data['clientType']);
        }
        elseif (\array_key_exists('clientType', $data) && $data['clientType'] === null) {
            $object->setClientType(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt($data['createdAt']);
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('createdBy', $data) && $data['createdBy'] !== null) {
            $object->setCreatedBy($data['createdBy']);
            unset($data['createdBy']);
        }
        elseif (\array_key_exists('createdBy', $data) && $data['createdBy'] === null) {
            $object->setCreatedBy(null);
        }
        if (\array_key_exists('defaultScopes', $data) && $data['defaultScopes'] !== null) {
            $values_2 = [];
            foreach ($data['defaultScopes'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setDefaultScopes($values_2);
            unset($data['defaultScopes']);
        }
        elseif (\array_key_exists('defaultScopes', $data) && $data['defaultScopes'] === null) {
            $object->setDefaultScopes(null);
        }
        if (\array_key_exists('grantTypes', $data) && $data['grantTypes'] !== null) {
            $values_3 = [];
            foreach ($data['grantTypes'] as $value_3) {
                $values_3[] = $value_3;
            }
            $object->setGrantTypes($values_3);
            unset($data['grantTypes']);
        }
        elseif (\array_key_exists('grantTypes', $data) && $data['grantTypes'] === null) {
            $object->setGrantTypes(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
            unset($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('pkceRequired', $data) && $data['pkceRequired'] !== null) {
            $object->setPkceRequired($data['pkceRequired']);
            unset($data['pkceRequired']);
        }
        elseif (\array_key_exists('pkceRequired', $data) && $data['pkceRequired'] === null) {
            $object->setPkceRequired(null);
        }
        if (\array_key_exists('redirectUris', $data) && $data['redirectUris'] !== null) {
            $values_4 = [];
            foreach ($data['redirectUris'] as $value_4) {
                $values_4[] = $value_4;
            }
            $object->setRedirectUris($values_4);
            unset($data['redirectUris']);
        }
        elseif (\array_key_exists('redirectUris', $data) && $data['redirectUris'] === null) {
            $object->setRedirectUris(null);
        }
        if (\array_key_exists('serviceAccountPrincipalId', $data) && $data['serviceAccountPrincipalId'] !== null) {
            $object->setServiceAccountPrincipalId($data['serviceAccountPrincipalId']);
            unset($data['serviceAccountPrincipalId']);
        }
        elseif (\array_key_exists('serviceAccountPrincipalId', $data) && $data['serviceAccountPrincipalId'] === null) {
            $object->setServiceAccountPrincipalId(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt($data['updatedAt']);
            unset($data['updatedAt']);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        foreach ($data as $key => $value_5) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_5;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['active'] = $data->getActive();
        if ($data->isInitialized('allowedOrigins') && null !== $data->getAllowedOrigins()) {
            $values = [];
            foreach ($data->getAllowedOrigins() as $value) {
                $values[] = $value;
            }
            $dataArray['allowedOrigins'] = $values;
        }
        $values_1 = [];
        foreach ($data->getApplicationIds() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['applicationIds'] = $values_1;
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['clientName'] = $data->getClientName();
        $dataArray['clientType'] = $data->getClientType();
        $dataArray['createdAt'] = $data->getCreatedAt();
        if ($data->isInitialized('createdBy')) {
            $dataArray['createdBy'] = $data->getCreatedBy();
        }
        $values_2 = [];
        foreach ($data->getDefaultScopes() as $value_2) {
            $values_2[] = $value_2;
        }
        $dataArray['defaultScopes'] = $values_2;
        $values_3 = [];
        foreach ($data->getGrantTypes() as $value_3) {
            $values_3[] = $value_3;
        }
        $dataArray['grantTypes'] = $values_3;
        $dataArray['id'] = $data->getId();
        $dataArray['pkceRequired'] = $data->getPkceRequired();
        $values_4 = [];
        foreach ($data->getRedirectUris() as $value_4) {
            $values_4[] = $value_4;
        }
        $dataArray['redirectUris'] = $values_4;
        if ($data->isInitialized('serviceAccountPrincipalId')) {
            $dataArray['serviceAccountPrincipalId'] = $data->getServiceAccountPrincipalId();
        }
        $dataArray['updatedAt'] = $data->getUpdatedAt();
        foreach ($data as $key => $value_5) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_5;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\OAuthClientResponse::class => false];
    }
}