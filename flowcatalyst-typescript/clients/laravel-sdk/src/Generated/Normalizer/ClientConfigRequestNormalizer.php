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
class ClientConfigRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ClientConfigRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ClientConfigRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ClientConfigRequest();
        if (\array_key_exists('enabled', $data) && \is_int($data['enabled'])) {
            $data['enabled'] = (bool) $data['enabled'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
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
        if ($data->isInitialized('enabled') && null !== $data->getEnabled()) {
            $dataArray['enabled'] = $data->getEnabled();
        }
        if ($data->isInitialized('baseUrlOverride') && null !== $data->getBaseUrlOverride()) {
            $dataArray['baseUrlOverride'] = $data->getBaseUrlOverride();
        }
        if ($data->isInitialized('websiteOverride') && null !== $data->getWebsiteOverride()) {
            $dataArray['websiteOverride'] = $data->getWebsiteOverride();
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
        return [\FlowCatalyst\Generated\Model\ClientConfigRequest::class => false];
    }
}