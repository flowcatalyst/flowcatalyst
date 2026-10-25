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
class IngestionSourceKindResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionSourceKindResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionSourceKindResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionSourceKindResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('builtIn', $data) && \is_int($data['builtIn'])) {
            $data['builtIn'] = (bool) $data['builtIn'];
        }
        if (\array_key_exists('singleOwner', $data) && \is_int($data['singleOwner'])) {
            $data['singleOwner'] = (bool) $data['singleOwner'];
        }
        if (\array_key_exists('builtIn', $data) && $data['builtIn'] !== null) {
            $object->setBuiltIn($data['builtIn']);
        }
        elseif (\array_key_exists('builtIn', $data) && $data['builtIn'] === null) {
            $object->setBuiltIn(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
        }
        if (\array_key_exists('requiredPermission', $data) && $data['requiredPermission'] !== null) {
            $object->setRequiredPermission($data['requiredPermission']);
        }
        elseif (\array_key_exists('requiredPermission', $data) && $data['requiredPermission'] === null) {
            $object->setRequiredPermission(null);
        }
        if (\array_key_exists('settings', $data) && $data['settings'] !== null) {
            $object->setSettings($data['settings']);
        }
        elseif (\array_key_exists('settings', $data) && $data['settings'] === null) {
            $object->setSettings(null);
        }
        if (\array_key_exists('singleOwner', $data) && $data['singleOwner'] !== null) {
            $object->setSingleOwner($data['singleOwner']);
        }
        elseif (\array_key_exists('singleOwner', $data) && $data['singleOwner'] === null) {
            $object->setSingleOwner(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['builtIn'] = $data->getBuiltIn();
        if ($data->isInitialized('description') && null !== $data->getDescription()) {
            $dataArray['description'] = $data->getDescription();
        }
        $dataArray['kind'] = $data->getKind();
        if ($data->isInitialized('requiredPermission') && null !== $data->getRequiredPermission()) {
            $dataArray['requiredPermission'] = $data->getRequiredPermission();
        }
        if ($data->isInitialized('settings') && null !== $data->getSettings()) {
            $dataArray['settings'] = $data->getSettings();
        }
        $dataArray['singleOwner'] = $data->getSingleOwner();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionSourceKindResponse::class => false];
    }
}