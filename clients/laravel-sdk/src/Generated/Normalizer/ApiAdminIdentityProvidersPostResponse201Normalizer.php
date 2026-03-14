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
class ApiAdminIdentityProvidersPostResponse201Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse201::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse201::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse201();
        if (\array_key_exists('hasClientSecret', $data) && \is_int($data['hasClientSecret'])) {
            $data['hasClientSecret'] = (bool) $data['hasClientSecret'];
        }
        if (\array_key_exists('oidcMultiTenant', $data) && \is_int($data['oidcMultiTenant'])) {
            $data['oidcMultiTenant'] = (bool) $data['oidcMultiTenant'];
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
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
            unset($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
            unset($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('type', $data) && $data['type'] !== null) {
            $object->setType($data['type']);
            unset($data['type']);
        }
        elseif (\array_key_exists('type', $data) && $data['type'] === null) {
            $object->setType(null);
        }
        if (\array_key_exists('oidcIssuerUrl', $data) && $data['oidcIssuerUrl'] !== null) {
            $object->setOidcIssuerUrl($data['oidcIssuerUrl']);
            unset($data['oidcIssuerUrl']);
        }
        elseif (\array_key_exists('oidcIssuerUrl', $data) && $data['oidcIssuerUrl'] === null) {
            $object->setOidcIssuerUrl(null);
        }
        if (\array_key_exists('oidcClientId', $data) && $data['oidcClientId'] !== null) {
            $object->setOidcClientId($data['oidcClientId']);
            unset($data['oidcClientId']);
        }
        elseif (\array_key_exists('oidcClientId', $data) && $data['oidcClientId'] === null) {
            $object->setOidcClientId(null);
        }
        if (\array_key_exists('hasClientSecret', $data) && $data['hasClientSecret'] !== null) {
            $object->setHasClientSecret($data['hasClientSecret']);
            unset($data['hasClientSecret']);
        }
        elseif (\array_key_exists('hasClientSecret', $data) && $data['hasClientSecret'] === null) {
            $object->setHasClientSecret(null);
        }
        if (\array_key_exists('oidcMultiTenant', $data) && $data['oidcMultiTenant'] !== null) {
            $object->setOidcMultiTenant($data['oidcMultiTenant']);
            unset($data['oidcMultiTenant']);
        }
        elseif (\array_key_exists('oidcMultiTenant', $data) && $data['oidcMultiTenant'] === null) {
            $object->setOidcMultiTenant(null);
        }
        if (\array_key_exists('oidcIssuerPattern', $data) && $data['oidcIssuerPattern'] !== null) {
            $object->setOidcIssuerPattern($data['oidcIssuerPattern']);
            unset($data['oidcIssuerPattern']);
        }
        elseif (\array_key_exists('oidcIssuerPattern', $data) && $data['oidcIssuerPattern'] === null) {
            $object->setOidcIssuerPattern(null);
        }
        if (\array_key_exists('allowedEmailDomains', $data) && $data['allowedEmailDomains'] !== null) {
            $values = [];
            foreach ($data['allowedEmailDomains'] as $value) {
                $values[] = $value;
            }
            $object->setAllowedEmailDomains($values);
            unset($data['allowedEmailDomains']);
        }
        elseif (\array_key_exists('allowedEmailDomains', $data) && $data['allowedEmailDomains'] === null) {
            $object->setAllowedEmailDomains(null);
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
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_1;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['id'] = $data->getId();
        $dataArray['code'] = $data->getCode();
        $dataArray['name'] = $data->getName();
        $dataArray['type'] = $data->getType();
        $dataArray['oidcIssuerUrl'] = $data->getOidcIssuerUrl();
        $dataArray['oidcClientId'] = $data->getOidcClientId();
        $dataArray['hasClientSecret'] = $data->getHasClientSecret();
        $dataArray['oidcMultiTenant'] = $data->getOidcMultiTenant();
        $dataArray['oidcIssuerPattern'] = $data->getOidcIssuerPattern();
        $values = [];
        foreach ($data->getAllowedEmailDomains() as $value) {
            $values[] = $value;
        }
        $dataArray['allowedEmailDomains'] = $values;
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse201::class => false];
    }
}