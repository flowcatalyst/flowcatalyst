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
class AppUpdateDetailResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AppUpdateDetailResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AppUpdateDetailResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\AppUpdateDetailResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('hasExpoClient', $data) && \is_int($data['hasExpoClient'])) {
            $data['hasExpoClient'] = (bool) $data['hasExpoClient'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('assets', $data) && $data['assets'] !== null) {
            $values = [];
            foreach ($data['assets'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\AppUpdateAssetResponse::class, 'json', $context);
            }
            $object->setAssets($values);
        }
        elseif (\array_key_exists('assets', $data) && $data['assets'] === null) {
            $object->setAssets(null);
        }
        if (\array_key_exists('hasExpoClient', $data) && $data['hasExpoClient'] !== null) {
            $object->setHasExpoClient($data['hasExpoClient']);
        }
        elseif (\array_key_exists('hasExpoClient', $data) && $data['hasExpoClient'] === null) {
            $object->setHasExpoClient(null);
        }
        if (\array_key_exists('totalBytes', $data) && $data['totalBytes'] !== null) {
            $object->setTotalBytes($data['totalBytes']);
        }
        elseif (\array_key_exists('totalBytes', $data) && $data['totalBytes'] === null) {
            $object->setTotalBytes(null);
        }
        if (\array_key_exists('update', $data) && $data['update'] !== null) {
            $object->setUpdate($this->denormalizer->denormalize($data['update'], \FlowCatalyst\Generated\Model\AppUpdateResponse::class, 'json', $context));
        }
        elseif (\array_key_exists('update', $data) && $data['update'] === null) {
            $object->setUpdate(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getAssets() as $value) {
            $values[] = $value === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value, 'json', $context));
        }
        $dataArray['assets'] = $values;
        $dataArray['hasExpoClient'] = $data->getHasExpoClient();
        $dataArray['totalBytes'] = $data->getTotalBytes();
        $dataArray['update'] = $data->getUpdate() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getUpdate(), 'json', $context));
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AppUpdateDetailResponse::class => false];
    }
}