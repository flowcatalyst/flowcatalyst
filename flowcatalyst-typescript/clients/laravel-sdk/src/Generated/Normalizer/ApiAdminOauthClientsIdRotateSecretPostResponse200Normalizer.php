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
class ApiAdminOauthClientsIdRotateSecretPostResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200();
        if (\array_key_exists('hasClientSecret', $data) && \is_int($data['hasClientSecret'])) {
            $data['hasClientSecret'] = (bool) $data['hasClientSecret'];
        }
        if (\array_key_exists('pkceRequired', $data) && \is_int($data['pkceRequired'])) {
            $data['pkceRequired'] = (bool) $data['pkceRequired'];
        }
        if (\array_key_exists('active', $data) && \is_int($data['active'])) {
            $data['active'] = (bool) $data['active'];
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
        if (\array_key_exists('hasClientSecret', $data) && $data['hasClientSecret'] !== null) {
            $object->setHasClientSecret($data['hasClientSecret']);
            unset($data['hasClientSecret']);
        }
        elseif (\array_key_exists('hasClientSecret', $data) && $data['hasClientSecret'] === null) {
            $object->setHasClientSecret(null);
        }
        if (\array_key_exists('redirectUris', $data) && $data['redirectUris'] !== null) {
            $values = [];
            foreach ($data['redirectUris'] as $value) {
                $values[] = $value;
            }
            $object->setRedirectUris($values);
            unset($data['redirectUris']);
        }
        elseif (\array_key_exists('redirectUris', $data) && $data['redirectUris'] === null) {
            $object->setRedirectUris(null);
        }
        if (\array_key_exists('allowedOrigins', $data) && $data['allowedOrigins'] !== null) {
            $values_1 = [];
            foreach ($data['allowedOrigins'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setAllowedOrigins($values_1);
            unset($data['allowedOrigins']);
        }
        elseif (\array_key_exists('allowedOrigins', $data) && $data['allowedOrigins'] === null) {
            $object->setAllowedOrigins(null);
        }
        if (\array_key_exists('grantTypes', $data) && $data['grantTypes'] !== null) {
            $values_2 = [];
            foreach ($data['grantTypes'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setGrantTypes($values_2);
            unset($data['grantTypes']);
        }
        elseif (\array_key_exists('grantTypes', $data) && $data['grantTypes'] === null) {
            $object->setGrantTypes(null);
        }
        if (\array_key_exists('defaultScopes', $data) && $data['defaultScopes'] !== null) {
            $values_3 = [];
            foreach ($data['defaultScopes'] as $value_3) {
                $values_3[] = $value_3;
            }
            $object->setDefaultScopes($values_3);
            unset($data['defaultScopes']);
        }
        elseif (\array_key_exists('defaultScopes', $data) && $data['defaultScopes'] === null) {
            $object->setDefaultScopes(null);
        }
        if (\array_key_exists('pkceRequired', $data) && $data['pkceRequired'] !== null) {
            $object->setPkceRequired($data['pkceRequired']);
            unset($data['pkceRequired']);
        }
        elseif (\array_key_exists('pkceRequired', $data) && $data['pkceRequired'] === null) {
            $object->setPkceRequired(null);
        }
        if (\array_key_exists('applicationIds', $data) && $data['applicationIds'] !== null) {
            $values_4 = [];
            foreach ($data['applicationIds'] as $value_4) {
                $values_4[] = $value_4;
            }
            $object->setApplicationIds($values_4);
            unset($data['applicationIds']);
        }
        elseif (\array_key_exists('applicationIds', $data) && $data['applicationIds'] === null) {
            $object->setApplicationIds(null);
        }
        if (\array_key_exists('applications', $data) && $data['applications'] !== null) {
            $values_5 = [];
            foreach ($data['applications'] as $value_5) {
                $values_5[] = $this->denormalizer->denormalize($value_5, \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200ApplicationsItem::class, 'json', $context);
            }
            $object->setApplications($values_5);
            unset($data['applications']);
        }
        elseif (\array_key_exists('applications', $data) && $data['applications'] === null) {
            $object->setApplications(null);
        }
        if (\array_key_exists('serviceAccountPrincipalId', $data) && $data['serviceAccountPrincipalId'] !== null) {
            $object->setServiceAccountPrincipalId($data['serviceAccountPrincipalId']);
            unset($data['serviceAccountPrincipalId']);
        }
        elseif (\array_key_exists('serviceAccountPrincipalId', $data) && $data['serviceAccountPrincipalId'] === null) {
            $object->setServiceAccountPrincipalId(null);
        }
        if (\array_key_exists('active', $data) && $data['active'] !== null) {
            $object->setActive($data['active']);
            unset($data['active']);
        }
        elseif (\array_key_exists('active', $data) && $data['active'] === null) {
            $object->setActive(null);
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
        foreach ($data as $key => $value_6) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_6;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['id'] = $data->getId();
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['clientName'] = $data->getClientName();
        $dataArray['clientType'] = $data->getClientType();
        $dataArray['hasClientSecret'] = $data->getHasClientSecret();
        $values = [];
        foreach ($data->getRedirectUris() as $value) {
            $values[] = $value;
        }
        $dataArray['redirectUris'] = $values;
        $values_1 = [];
        foreach ($data->getAllowedOrigins() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['allowedOrigins'] = $values_1;
        $values_2 = [];
        foreach ($data->getGrantTypes() as $value_2) {
            $values_2[] = $value_2;
        }
        $dataArray['grantTypes'] = $values_2;
        $values_3 = [];
        foreach ($data->getDefaultScopes() as $value_3) {
            $values_3[] = $value_3;
        }
        $dataArray['defaultScopes'] = $values_3;
        $dataArray['pkceRequired'] = $data->getPkceRequired();
        $values_4 = [];
        foreach ($data->getApplicationIds() as $value_4) {
            $values_4[] = $value_4;
        }
        $dataArray['applicationIds'] = $values_4;
        $values_5 = [];
        foreach ($data->getApplications() as $value_5) {
            $values_5[] = $this->normalizer->normalize($value_5, 'json', $context);
        }
        $dataArray['applications'] = $values_5;
        $dataArray['serviceAccountPrincipalId'] = $data->getServiceAccountPrincipalId();
        $dataArray['active'] = $data->getActive();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value_6) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_6;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200::class => false];
    }
}