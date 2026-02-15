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
class ClientConfigResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ClientConfigResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ClientConfigResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ClientConfigResponse();
        if (\array_key_exists('enabled', $data) && \is_int($data['enabled'])) {
            $data['enabled'] = (bool) $data['enabled'];
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
        if (\array_key_exists('applicationId', $data) && $data['applicationId'] !== null) {
            $object->setApplicationId($data['applicationId']);
            unset($data['applicationId']);
        }
        elseif (\array_key_exists('applicationId', $data) && $data['applicationId'] === null) {
            $object->setApplicationId(null);
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
        if (\array_key_exists('clientIdentifier', $data) && $data['clientIdentifier'] !== null) {
            $object->setClientIdentifier($data['clientIdentifier']);
            unset($data['clientIdentifier']);
        }
        elseif (\array_key_exists('clientIdentifier', $data) && $data['clientIdentifier'] === null) {
            $object->setClientIdentifier(null);
        }
        if (\array_key_exists('enabled', $data) && $data['enabled'] !== null) {
            $object->setEnabled($data['enabled']);
            unset($data['enabled']);
        }
        elseif (\array_key_exists('enabled', $data) && $data['enabled'] === null) {
            $object->setEnabled(null);
        }
        if (\array_key_exists('baseUrlOverride', $data) && $data['baseUrlOverride'] !== null) {
            $object->setBaseUrlOverride($data['baseUrlOverride']);
            unset($data['baseUrlOverride']);
        }
        elseif (\array_key_exists('baseUrlOverride', $data) && $data['baseUrlOverride'] === null) {
            $object->setBaseUrlOverride(null);
        }
        if (\array_key_exists('websiteOverride', $data) && $data['websiteOverride'] !== null) {
            $object->setWebsiteOverride($data['websiteOverride']);
            unset($data['websiteOverride']);
        }
        elseif (\array_key_exists('websiteOverride', $data) && $data['websiteOverride'] === null) {
            $object->setWebsiteOverride(null);
        }
        if (\array_key_exists('effectiveBaseUrl', $data) && $data['effectiveBaseUrl'] !== null) {
            $object->setEffectiveBaseUrl($data['effectiveBaseUrl']);
            unset($data['effectiveBaseUrl']);
        }
        elseif (\array_key_exists('effectiveBaseUrl', $data) && $data['effectiveBaseUrl'] === null) {
            $object->setEffectiveBaseUrl(null);
        }
        if (\array_key_exists('effectiveWebsite', $data) && $data['effectiveWebsite'] !== null) {
            $object->setEffectiveWebsite($data['effectiveWebsite']);
            unset($data['effectiveWebsite']);
        }
        elseif (\array_key_exists('effectiveWebsite', $data) && $data['effectiveWebsite'] === null) {
            $object->setEffectiveWebsite(null);
        }
        if (\array_key_exists('config', $data) && $data['config'] !== null) {
            $values = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['config'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setConfig($values);
            unset($data['config']);
        }
        elseif (\array_key_exists('config', $data) && $data['config'] === null) {
            $object->setConfig(null);
        }
        foreach ($data as $key_1 => $value_1) {
            if (preg_match('/.*/', (string) $key_1)) {
                $object[$key_1] = $value_1;
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
        if ($data->isInitialized('applicationId') && null !== $data->getApplicationId()) {
            $dataArray['applicationId'] = $data->getApplicationId();
        }
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['clientId'] = $data->getClientId();
        }
        if ($data->isInitialized('clientName') && null !== $data->getClientName()) {
            $dataArray['clientName'] = $data->getClientName();
        }
        if ($data->isInitialized('clientIdentifier') && null !== $data->getClientIdentifier()) {
            $dataArray['clientIdentifier'] = $data->getClientIdentifier();
        }
        if ($data->isInitialized('enabled') && null !== $data->getEnabled()) {
            $dataArray['enabled'] = $data->getEnabled();
        }
        if ($data->isInitialized('baseUrlOverride') && null !== $data->getBaseUrlOverride()) {
            $dataArray['baseUrlOverride'] = $data->getBaseUrlOverride();
        }
        if ($data->isInitialized('websiteOverride') && null !== $data->getWebsiteOverride()) {
            $dataArray['websiteOverride'] = $data->getWebsiteOverride();
        }
        if ($data->isInitialized('effectiveBaseUrl') && null !== $data->getEffectiveBaseUrl()) {
            $dataArray['effectiveBaseUrl'] = $data->getEffectiveBaseUrl();
        }
        if ($data->isInitialized('effectiveWebsite') && null !== $data->getEffectiveWebsite()) {
            $dataArray['effectiveWebsite'] = $data->getEffectiveWebsite();
        }
        if ($data->isInitialized('config') && null !== $data->getConfig()) {
            $values = [];
            foreach ($data->getConfig() as $key => $value) {
                $values[$key] = $value;
            }
            $dataArray['config'] = $values;
        }
        foreach ($data as $key_1 => $value_1) {
            if (preg_match('/.*/', (string) $key_1)) {
                $dataArray[$key_1] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ClientConfigResponse::class => false];
    }
}