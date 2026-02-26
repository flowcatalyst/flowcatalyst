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
class ApiAdminConfigAppCodeSectionPropertyPutResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200();
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
        if (\array_key_exists('valueType', $data) && $data['valueType'] !== null) {
            $object->setValueType($data['valueType']);
            unset($data['valueType']);
        }
        elseif (\array_key_exists('valueType', $data) && $data['valueType'] === null) {
            $object->setValueType(null);
        }
        if (\array_key_exists('value', $data) && $data['value'] !== null) {
            $object->setValue($data['value']);
            unset($data['value']);
        }
        elseif (\array_key_exists('value', $data) && $data['value'] === null) {
            $object->setValue(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
            unset($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
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
        $dataArray['id'] = $data->getId();
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['section'] = $data->getSection();
        $dataArray['property'] = $data->getProperty();
        $dataArray['scope'] = $data->getScope();
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['valueType'] = $data->getValueType();
        $dataArray['value'] = $data->getValue();
        $dataArray['description'] = $data->getDescription();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200::class => false];
    }
}