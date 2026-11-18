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
class AppUpdateBranchResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AppUpdateBranchResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AppUpdateBranchResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\AppUpdateBranchResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('appKey', $data) && $data['appKey'] !== null) {
            $object->setAppKey($data['appKey']);
        }
        elseif (\array_key_exists('appKey', $data) && $data['appKey'] === null) {
            $object->setAppKey(null);
        }
        if (\array_key_exists('channel', $data) && $data['channel'] !== null) {
            $object->setChannel($data['channel']);
        }
        elseif (\array_key_exists('channel', $data) && $data['channel'] === null) {
            $object->setChannel(null);
        }
        if (\array_key_exists('current', $data) && $data['current'] !== null) {
            $object->setCurrent($this->denormalizer->denormalize($data['current'], \FlowCatalyst\Generated\Model\AppUpdateResponse::class, 'json', $context));
        }
        elseif (\array_key_exists('current', $data) && $data['current'] === null) {
            $object->setCurrent(null);
        }
        if (\array_key_exists('platform', $data) && $data['platform'] !== null) {
            $object->setPlatform($data['platform']);
        }
        elseif (\array_key_exists('platform', $data) && $data['platform'] === null) {
            $object->setPlatform(null);
        }
        if (\array_key_exists('previous', $data) && $data['previous'] !== null) {
            $object->setPrevious($this->denormalizer->denormalize($data['previous'], \FlowCatalyst\Generated\Model\AppUpdateResponse::class, 'json', $context));
        }
        elseif (\array_key_exists('previous', $data) && $data['previous'] === null) {
            $object->setPrevious(null);
        }
        if (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] !== null) {
            $object->setRuntimeVersion($data['runtimeVersion']);
        }
        elseif (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] === null) {
            $object->setRuntimeVersion(null);
        }
        if (\array_key_exists('updateCount', $data) && $data['updateCount'] !== null) {
            $object->setUpdateCount($data['updateCount']);
        }
        elseif (\array_key_exists('updateCount', $data) && $data['updateCount'] === null) {
            $object->setUpdateCount(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['appKey'] = $data->getAppKey();
        $dataArray['channel'] = $data->getChannel();
        $dataArray['current'] = $data->getCurrent() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getCurrent(), 'json', $context));
        $dataArray['platform'] = $data->getPlatform();
        if ($data->isInitialized('previous') && null !== $data->getPrevious()) {
            $dataArray['previous'] = $data->getPrevious() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getPrevious(), 'json', $context));
        }
        $dataArray['runtimeVersion'] = $data->getRuntimeVersion();
        $dataArray['updateCount'] = $data->getUpdateCount();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AppUpdateBranchResponse::class => false];
    }
}