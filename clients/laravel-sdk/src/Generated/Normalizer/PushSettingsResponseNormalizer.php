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
class PushSettingsResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PushSettingsResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PushSettingsResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PushSettingsResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('effective', $data) && $data['effective'] !== null) {
            $object->setEffective($this->denormalizer->denormalize($data['effective'], \FlowCatalyst\Generated\Model\PushSettingsResponseEffectiveStruct::class, 'json', $context));
        }
        elseif (\array_key_exists('effective', $data) && $data['effective'] === null) {
            $object->setEffective(null);
        }
        if (\array_key_exists('settings', $data) && $data['settings'] !== null) {
            $object->setSettings($this->denormalizer->denormalize($data['settings'], \FlowCatalyst\Generated\Model\PushSettings::class, 'json', $context));
        }
        elseif (\array_key_exists('settings', $data) && $data['settings'] === null) {
            $object->setSettings(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']);
            if (false === $date) {
                if (is_string($data['updatedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['updatedAt'])) {
                    try {
                        $date = new \DateTime($data['updatedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setUpdatedAt($date);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        if (\array_key_exists('updatedBy', $data) && $data['updatedBy'] !== null) {
            $object->setUpdatedBy($data['updatedBy']);
        }
        elseif (\array_key_exists('updatedBy', $data) && $data['updatedBy'] === null) {
            $object->setUpdatedBy(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['effective'] = $data->getEffective() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getEffective(), 'json', $context));
        $dataArray['settings'] = $data->getSettings() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getSettings(), 'json', $context));
        if ($data->isInitialized('updatedAt') && null !== $data->getUpdatedAt()) {
            $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('updatedBy') && null !== $data->getUpdatedBy()) {
            $dataArray['updatedBy'] = $data->getUpdatedBy();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PushSettingsResponse::class => false];
    }
}