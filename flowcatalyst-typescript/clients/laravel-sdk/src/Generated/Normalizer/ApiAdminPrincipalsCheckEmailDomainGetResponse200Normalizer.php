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
class ApiAdminPrincipalsCheckEmailDomainGetResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse200();
        if (\array_key_exists('isAnchorDomain', $data) && \is_int($data['isAnchorDomain'])) {
            $data['isAnchorDomain'] = (bool) $data['isAnchorDomain'];
        }
        if (\array_key_exists('hasAuthConfig', $data) && \is_int($data['hasAuthConfig'])) {
            $data['hasAuthConfig'] = (bool) $data['hasAuthConfig'];
        }
        if (\array_key_exists('emailExists', $data) && \is_int($data['emailExists'])) {
            $data['emailExists'] = (bool) $data['emailExists'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('domain', $data) && $data['domain'] !== null) {
            $object->setDomain($data['domain']);
            unset($data['domain']);
        }
        elseif (\array_key_exists('domain', $data) && $data['domain'] === null) {
            $object->setDomain(null);
        }
        if (\array_key_exists('authProvider', $data) && $data['authProvider'] !== null) {
            $object->setAuthProvider($data['authProvider']);
            unset($data['authProvider']);
        }
        elseif (\array_key_exists('authProvider', $data) && $data['authProvider'] === null) {
            $object->setAuthProvider(null);
        }
        if (\array_key_exists('isAnchorDomain', $data) && $data['isAnchorDomain'] !== null) {
            $object->setIsAnchorDomain($data['isAnchorDomain']);
            unset($data['isAnchorDomain']);
        }
        elseif (\array_key_exists('isAnchorDomain', $data) && $data['isAnchorDomain'] === null) {
            $object->setIsAnchorDomain(null);
        }
        if (\array_key_exists('hasAuthConfig', $data) && $data['hasAuthConfig'] !== null) {
            $object->setHasAuthConfig($data['hasAuthConfig']);
            unset($data['hasAuthConfig']);
        }
        elseif (\array_key_exists('hasAuthConfig', $data) && $data['hasAuthConfig'] === null) {
            $object->setHasAuthConfig(null);
        }
        if (\array_key_exists('emailExists', $data) && $data['emailExists'] !== null) {
            $object->setEmailExists($data['emailExists']);
            unset($data['emailExists']);
        }
        elseif (\array_key_exists('emailExists', $data) && $data['emailExists'] === null) {
            $object->setEmailExists(null);
        }
        if (\array_key_exists('info', $data) && $data['info'] !== null) {
            $object->setInfo($data['info']);
            unset($data['info']);
        }
        elseif (\array_key_exists('info', $data) && $data['info'] === null) {
            $object->setInfo(null);
        }
        if (\array_key_exists('warning', $data) && $data['warning'] !== null) {
            $object->setWarning($data['warning']);
            unset($data['warning']);
        }
        elseif (\array_key_exists('warning', $data) && $data['warning'] === null) {
            $object->setWarning(null);
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
        $dataArray['domain'] = $data->getDomain();
        $dataArray['authProvider'] = $data->getAuthProvider();
        $dataArray['isAnchorDomain'] = $data->getIsAnchorDomain();
        $dataArray['hasAuthConfig'] = $data->getHasAuthConfig();
        $dataArray['emailExists'] = $data->getEmailExists();
        $dataArray['info'] = $data->getInfo();
        $dataArray['warning'] = $data->getWarning();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse200::class => false];
    }
}