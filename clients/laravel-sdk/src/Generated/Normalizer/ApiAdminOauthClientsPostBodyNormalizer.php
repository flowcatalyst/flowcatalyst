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
class ApiAdminOauthClientsPostBodyNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody();
        if (\array_key_exists('pkceRequired', $data) && \is_int($data['pkceRequired'])) {
            $data['pkceRequired'] = (bool) $data['pkceRequired'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
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
        if (\array_key_exists('clientSecretRef', $data) && $data['clientSecretRef'] !== null) {
            $object->setClientSecretRef($data['clientSecretRef']);
            unset($data['clientSecretRef']);
        }
        elseif (\array_key_exists('clientSecretRef', $data) && $data['clientSecretRef'] === null) {
            $object->setClientSecretRef(null);
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
            $object->setDefaultScopes($data['defaultScopes']);
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
            $values_3 = [];
            foreach ($data['applicationIds'] as $value_3) {
                $values_3[] = $value_3;
            }
            $object->setApplicationIds($values_3);
            unset($data['applicationIds']);
        }
        elseif (\array_key_exists('applicationIds', $data) && $data['applicationIds'] === null) {
            $object->setApplicationIds(null);
        }
        foreach ($data as $key => $value_4) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_4;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['clientName'] = $data->getClientName();
        $dataArray['clientType'] = $data->getClientType();
        if ($data->isInitialized('clientSecretRef') && null !== $data->getClientSecretRef()) {
            $dataArray['clientSecretRef'] = $data->getClientSecretRef();
        }
        if ($data->isInitialized('redirectUris') && null !== $data->getRedirectUris()) {
            $values = [];
            foreach ($data->getRedirectUris() as $value) {
                $values[] = $value;
            }
            $dataArray['redirectUris'] = $values;
        }
        if ($data->isInitialized('allowedOrigins') && null !== $data->getAllowedOrigins()) {
            $values_1 = [];
            foreach ($data->getAllowedOrigins() as $value_1) {
                $values_1[] = $value_1;
            }
            $dataArray['allowedOrigins'] = $values_1;
        }
        if ($data->isInitialized('grantTypes') && null !== $data->getGrantTypes()) {
            $values_2 = [];
            foreach ($data->getGrantTypes() as $value_2) {
                $values_2[] = $value_2;
            }
            $dataArray['grantTypes'] = $values_2;
        }
        if ($data->isInitialized('defaultScopes') && null !== $data->getDefaultScopes()) {
            $dataArray['defaultScopes'] = $data->getDefaultScopes();
        }
        if ($data->isInitialized('pkceRequired') && null !== $data->getPkceRequired()) {
            $dataArray['pkceRequired'] = $data->getPkceRequired();
        }
        if ($data->isInitialized('applicationIds') && null !== $data->getApplicationIds()) {
            $values_3 = [];
            foreach ($data->getApplicationIds() as $value_3) {
                $values_3[] = $value_3;
            }
            $dataArray['applicationIds'] = $values_3;
        }
        foreach ($data as $key => $value_4) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_4;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody::class => false];
    }
}