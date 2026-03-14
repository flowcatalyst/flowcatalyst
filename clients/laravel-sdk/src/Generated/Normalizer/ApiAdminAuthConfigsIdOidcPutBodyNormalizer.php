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
class ApiAdminAuthConfigsIdOidcPutBodyNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody();
        if (\array_key_exists('oidcMultiTenant', $data) && \is_int($data['oidcMultiTenant'])) {
            $data['oidcMultiTenant'] = (bool) $data['oidcMultiTenant'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
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
        if (\array_key_exists('oidcClientSecretRef', $data) && $data['oidcClientSecretRef'] !== null) {
            $object->setOidcClientSecretRef($data['oidcClientSecretRef']);
            unset($data['oidcClientSecretRef']);
        }
        elseif (\array_key_exists('oidcClientSecretRef', $data) && $data['oidcClientSecretRef'] === null) {
            $object->setOidcClientSecretRef(null);
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
        $dataArray['oidcIssuerUrl'] = $data->getOidcIssuerUrl();
        $dataArray['oidcClientId'] = $data->getOidcClientId();
        if ($data->isInitialized('oidcClientSecretRef') && null !== $data->getOidcClientSecretRef()) {
            $dataArray['oidcClientSecretRef'] = $data->getOidcClientSecretRef();
        }
        if ($data->isInitialized('oidcMultiTenant') && null !== $data->getOidcMultiTenant()) {
            $dataArray['oidcMultiTenant'] = $data->getOidcMultiTenant();
        }
        if ($data->isInitialized('oidcIssuerPattern') && null !== $data->getOidcIssuerPattern()) {
            $dataArray['oidcIssuerPattern'] = $data->getOidcIssuerPattern();
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
        return [\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody::class => false];
    }
}