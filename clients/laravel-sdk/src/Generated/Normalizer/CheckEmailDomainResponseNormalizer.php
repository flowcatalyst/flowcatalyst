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
class CheckEmailDomainResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\CheckEmailDomainResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\CheckEmailDomainResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\CheckEmailDomainResponse();
        if (\array_key_exists('emailExists', $data) && \is_int($data['emailExists'])) {
            $data['emailExists'] = (bool) $data['emailExists'];
        }
        if (\array_key_exists('hasAuthConfig', $data) && \is_int($data['hasAuthConfig'])) {
            $data['hasAuthConfig'] = (bool) $data['hasAuthConfig'];
        }
        if (\array_key_exists('isAnchorDomain', $data) && \is_int($data['isAnchorDomain'])) {
            $data['isAnchorDomain'] = (bool) $data['isAnchorDomain'];
        }
        if (\array_key_exists('requiresClientId', $data) && \is_int($data['requiresClientId'])) {
            $data['requiresClientId'] = (bool) $data['requiresClientId'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('allowedClientIds', $data) && $data['allowedClientIds'] !== null) {
            $values = [];
            foreach ($data['allowedClientIds'] as $value) {
                $values[] = $value;
            }
            $object->setAllowedClientIds($values);
            unset($data['allowedClientIds']);
        }
        elseif (\array_key_exists('allowedClientIds', $data) && $data['allowedClientIds'] === null) {
            $object->setAllowedClientIds(null);
        }
        if (\array_key_exists('authProvider', $data) && $data['authProvider'] !== null) {
            $object->setAuthProvider($data['authProvider']);
            unset($data['authProvider']);
        }
        elseif (\array_key_exists('authProvider', $data) && $data['authProvider'] === null) {
            $object->setAuthProvider(null);
        }
        if (\array_key_exists('derivedScope', $data) && $data['derivedScope'] !== null) {
            $object->setDerivedScope($data['derivedScope']);
            unset($data['derivedScope']);
        }
        elseif (\array_key_exists('derivedScope', $data) && $data['derivedScope'] === null) {
            $object->setDerivedScope(null);
        }
        if (\array_key_exists('domain', $data) && $data['domain'] !== null) {
            $object->setDomain($data['domain']);
            unset($data['domain']);
        }
        elseif (\array_key_exists('domain', $data) && $data['domain'] === null) {
            $object->setDomain(null);
        }
        if (\array_key_exists('emailExists', $data) && $data['emailExists'] !== null) {
            $object->setEmailExists($data['emailExists']);
            unset($data['emailExists']);
        }
        elseif (\array_key_exists('emailExists', $data) && $data['emailExists'] === null) {
            $object->setEmailExists(null);
        }
        if (\array_key_exists('hasAuthConfig', $data) && $data['hasAuthConfig'] !== null) {
            $object->setHasAuthConfig($data['hasAuthConfig']);
            unset($data['hasAuthConfig']);
        }
        elseif (\array_key_exists('hasAuthConfig', $data) && $data['hasAuthConfig'] === null) {
            $object->setHasAuthConfig(null);
        }
        if (\array_key_exists('info', $data) && $data['info'] !== null) {
            $object->setInfo($data['info']);
            unset($data['info']);
        }
        elseif (\array_key_exists('info', $data) && $data['info'] === null) {
            $object->setInfo(null);
        }
        if (\array_key_exists('isAnchorDomain', $data) && $data['isAnchorDomain'] !== null) {
            $object->setIsAnchorDomain($data['isAnchorDomain']);
            unset($data['isAnchorDomain']);
        }
        elseif (\array_key_exists('isAnchorDomain', $data) && $data['isAnchorDomain'] === null) {
            $object->setIsAnchorDomain(null);
        }
        if (\array_key_exists('requiresClientId', $data) && $data['requiresClientId'] !== null) {
            $object->setRequiresClientId($data['requiresClientId']);
            unset($data['requiresClientId']);
        }
        elseif (\array_key_exists('requiresClientId', $data) && $data['requiresClientId'] === null) {
            $object->setRequiresClientId(null);
        }
        if (\array_key_exists('warning', $data) && $data['warning'] !== null) {
            $object->setWarning($data['warning']);
            unset($data['warning']);
        }
        elseif (\array_key_exists('warning', $data) && $data['warning'] === null) {
            $object->setWarning(null);
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
        $values = [];
        foreach ($data->getAllowedClientIds() as $value) {
            $values[] = $value;
        }
        $dataArray['allowedClientIds'] = $values;
        if ($data->isInitialized('authProvider')) {
            $dataArray['authProvider'] = $data->getAuthProvider();
        }
        $dataArray['derivedScope'] = $data->getDerivedScope();
        $dataArray['domain'] = $data->getDomain();
        $dataArray['emailExists'] = $data->getEmailExists();
        $dataArray['hasAuthConfig'] = $data->getHasAuthConfig();
        if ($data->isInitialized('info')) {
            $dataArray['info'] = $data->getInfo();
        }
        $dataArray['isAnchorDomain'] = $data->getIsAnchorDomain();
        $dataArray['requiresClientId'] = $data->getRequiresClientId();
        if ($data->isInitialized('warning')) {
            $dataArray['warning'] = $data->getWarning();
        }
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\CheckEmailDomainResponse::class => false];
    }
}