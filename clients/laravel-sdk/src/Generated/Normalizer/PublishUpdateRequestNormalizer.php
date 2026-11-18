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
class PublishUpdateRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PublishUpdateRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PublishUpdateRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PublishUpdateRequest();
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
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
            unset($data['$schema']);
        }
        if (\array_key_exists('appKey', $data) && $data['appKey'] !== null) {
            $object->setAppKey($data['appKey']);
            unset($data['appKey']);
        }
        elseif (\array_key_exists('appKey', $data) && $data['appKey'] === null) {
            $object->setAppKey(null);
            unset($data['appKey']);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
            unset($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
            unset($data['applicationCode']);
        }
        if (\array_key_exists('assets', $data) && $data['assets'] !== null) {
            $values = [];
            foreach ($data['assets'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\PublishAssetRequest::class, 'json', $context);
            }
            $object->setAssets($values);
            unset($data['assets']);
        }
        elseif (\array_key_exists('assets', $data) && $data['assets'] === null) {
            $object->setAssets(null);
            unset($data['assets']);
        }
        if (\array_key_exists('channel', $data) && $data['channel'] !== null) {
            $object->setChannel($data['channel']);
            unset($data['channel']);
        }
        elseif (\array_key_exists('channel', $data) && $data['channel'] === null) {
            $object->setChannel(null);
            unset($data['channel']);
        }
        if (\array_key_exists('expoClient', $data) && $data['expoClient'] !== null) {
            $object->setExpoClient($data['expoClient']);
            unset($data['expoClient']);
        }
        elseif (\array_key_exists('expoClient', $data) && $data['expoClient'] === null) {
            $object->setExpoClient(null);
            unset($data['expoClient']);
        }
        if (\array_key_exists('launchAsset', $data) && $data['launchAsset'] !== null) {
            $object->setLaunchAsset($this->denormalizer->denormalize($data['launchAsset'], \FlowCatalyst\Generated\Model\PublishLaunchAssetRequest::class, 'json', $context));
            unset($data['launchAsset']);
        }
        elseif (\array_key_exists('launchAsset', $data) && $data['launchAsset'] === null) {
            $object->setLaunchAsset(null);
            unset($data['launchAsset']);
        }
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
            unset($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
            unset($data['message']);
        }
        if (\array_key_exists('platform', $data) && $data['platform'] !== null) {
            $object->setPlatform($data['platform']);
            unset($data['platform']);
        }
        elseif (\array_key_exists('platform', $data) && $data['platform'] === null) {
            $object->setPlatform(null);
            unset($data['platform']);
        }
        if (\array_key_exists('rolloutPercent', $data) && $data['rolloutPercent'] !== null) {
            $object->setRolloutPercent($data['rolloutPercent']);
            unset($data['rolloutPercent']);
        }
        elseif (\array_key_exists('rolloutPercent', $data) && $data['rolloutPercent'] === null) {
            $object->setRolloutPercent(null);
            unset($data['rolloutPercent']);
        }
        if (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] !== null) {
            $object->setRuntimeVersion($data['runtimeVersion']);
            unset($data['runtimeVersion']);
        }
        elseif (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] === null) {
            $object->setRuntimeVersion(null);
            unset($data['runtimeVersion']);
        }
        if (\array_key_exists('sourceCommit', $data) && $data['sourceCommit'] !== null) {
            $object->setSourceCommit($data['sourceCommit']);
            unset($data['sourceCommit']);
        }
        elseif (\array_key_exists('sourceCommit', $data) && $data['sourceCommit'] === null) {
            $object->setSourceCommit(null);
            unset($data['sourceCommit']);
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
        $dataArray['appKey'] = $data->getAppKey();
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $values = [];
        foreach ($data->getAssets() as $value) {
            $values[] = $value === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value, 'json', $context));
        }
        $dataArray['assets'] = $values;
        $dataArray['channel'] = $data->getChannel();
        if ($data->isInitialized('expoClient') && null !== $data->getExpoClient()) {
            $dataArray['expoClient'] = $data->getExpoClient();
        }
        $dataArray['launchAsset'] = $data->getLaunchAsset() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getLaunchAsset(), 'json', $context));
        if ($data->isInitialized('message') && null !== $data->getMessage()) {
            $dataArray['message'] = $data->getMessage();
        }
        $dataArray['platform'] = $data->getPlatform();
        if ($data->isInitialized('rolloutPercent') && null !== $data->getRolloutPercent()) {
            $dataArray['rolloutPercent'] = $data->getRolloutPercent();
        }
        $dataArray['runtimeVersion'] = $data->getRuntimeVersion();
        if ($data->isInitialized('sourceCommit') && null !== $data->getSourceCommit()) {
            $dataArray['sourceCommit'] = $data->getSourceCommit();
        }
        foreach ($data->additionalPropertyEntries() as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PublishUpdateRequest::class => false];
    }
}