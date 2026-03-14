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
class ApiAdminConfigAppCodeSectionPropertyGetResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
            unset($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('section', $data) && $data['section'] !== null) {
            $object->setSection($data['section']);
            unset($data['section']);
        }
        elseif (\array_key_exists('section', $data) && $data['section'] === null) {
            $object->setSection(null);
        }
        if (\array_key_exists('property', $data) && $data['property'] !== null) {
            $object->setProperty($data['property']);
            unset($data['property']);
        }
        elseif (\array_key_exists('property', $data) && $data['property'] === null) {
            $object->setProperty(null);
        }
        if (\array_key_exists('scope', $data) && $data['scope'] !== null) {
            $object->setScope($data['scope']);
            unset($data['scope']);
        }
        elseif (\array_key_exists('scope', $data) && $data['scope'] === null) {
            $object->setScope(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
            unset($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('value', $data) && $data['value'] !== null) {
            $object->setValue($data['value']);
            unset($data['value']);
        }
        elseif (\array_key_exists('value', $data) && $data['value'] === null) {
            $object->setValue(null);
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
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['section'] = $data->getSection();
        $dataArray['property'] = $data->getProperty();
        $dataArray['scope'] = $data->getScope();
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['value'] = $data->getValue();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200::class => false];
    }
}