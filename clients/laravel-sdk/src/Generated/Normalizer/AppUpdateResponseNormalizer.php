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
class AppUpdateResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AppUpdateResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AppUpdateResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\AppUpdateResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('live', $data) && \is_int($data['live'])) {
            $data['live'] = (bool) $data['live'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('appKey', $data) && $data['appKey'] !== null) {
            $object->setAppKey($data['appKey']);
        }
        elseif (\array_key_exists('appKey', $data) && $data['appKey'] === null) {
            $object->setAppKey(null);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('applicationId', $data) && $data['applicationId'] !== null) {
            $object->setApplicationId($data['applicationId']);
        }
        elseif (\array_key_exists('applicationId', $data) && $data['applicationId'] === null) {
            $object->setApplicationId(null);
        }
        if (\array_key_exists('channel', $data) && $data['channel'] !== null) {
            $object->setChannel($data['channel']);
        }
        elseif (\array_key_exists('channel', $data) && $data['channel'] === null) {
            $object->setChannel(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']);
            if (false === $date) {
                if (is_string($data['createdAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['createdAt'])) {
                    try {
                        $date = new \DateTime($data['createdAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['createdAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['createdAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setCreatedAt($date);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
        }
        if (\array_key_exists('live', $data) && $data['live'] !== null) {
            $object->setLive($data['live']);
        }
        elseif (\array_key_exists('live', $data) && $data['live'] === null) {
            $object->setLive(null);
        }
        if (\array_key_exists('manifestId', $data) && $data['manifestId'] !== null) {
            $object->setManifestId($data['manifestId']);
        }
        elseif (\array_key_exists('manifestId', $data) && $data['manifestId'] === null) {
            $object->setManifestId(null);
        }
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
        }
        if (\array_key_exists('platform', $data) && $data['platform'] !== null) {
            $object->setPlatform($data['platform']);
        }
        elseif (\array_key_exists('platform', $data) && $data['platform'] === null) {
            $object->setPlatform(null);
        }
        if (\array_key_exists('publishedBy', $data) && $data['publishedBy'] !== null) {
            $object->setPublishedBy($data['publishedBy']);
        }
        elseif (\array_key_exists('publishedBy', $data) && $data['publishedBy'] === null) {
            $object->setPublishedBy(null);
        }
        if (\array_key_exists('publishedByName', $data) && $data['publishedByName'] !== null) {
            $object->setPublishedByName($data['publishedByName']);
        }
        elseif (\array_key_exists('publishedByName', $data) && $data['publishedByName'] === null) {
            $object->setPublishedByName(null);
        }
        if (\array_key_exists('republishedFrom', $data) && $data['republishedFrom'] !== null) {
            $object->setRepublishedFrom($data['republishedFrom']);
        }
        elseif (\array_key_exists('republishedFrom', $data) && $data['republishedFrom'] === null) {
            $object->setRepublishedFrom(null);
        }
        if (\array_key_exists('rolloutPercent', $data) && $data['rolloutPercent'] !== null) {
            $object->setRolloutPercent($data['rolloutPercent']);
        }
        elseif (\array_key_exists('rolloutPercent', $data) && $data['rolloutPercent'] === null) {
            $object->setRolloutPercent(null);
        }
        if (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] !== null) {
            $object->setRuntimeVersion($data['runtimeVersion']);
        }
        elseif (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] === null) {
            $object->setRuntimeVersion(null);
        }
        if (\array_key_exists('servedPercent', $data) && $data['servedPercent'] !== null) {
            $object->setServedPercent($data['servedPercent']);
        }
        elseif (\array_key_exists('servedPercent', $data) && $data['servedPercent'] === null) {
            $object->setServedPercent(null);
        }
        if (\array_key_exists('sourceCommit', $data) && $data['sourceCommit'] !== null) {
            $object->setSourceCommit($data['sourceCommit']);
        }
        elseif (\array_key_exists('sourceCommit', $data) && $data['sourceCommit'] === null) {
            $object->setSourceCommit(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']);
            if (false === $date_1) {
                if (is_string($data['updatedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['updatedAt'])) {
                    try {
                        $date_1 = new \DateTime($data['updatedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['updatedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setUpdatedAt($date_1);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['appKey'] = $data->getAppKey();
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['applicationId'] = $data->getApplicationId();
        $dataArray['channel'] = $data->getChannel();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['id'] = $data->getId();
        $dataArray['kind'] = $data->getKind();
        $dataArray['live'] = $data->getLive();
        if ($data->isInitialized('manifestId') && null !== $data->getManifestId()) {
            $dataArray['manifestId'] = $data->getManifestId();
        }
        if ($data->isInitialized('message') && null !== $data->getMessage()) {
            $dataArray['message'] = $data->getMessage();
        }
        $dataArray['platform'] = $data->getPlatform();
        $dataArray['publishedBy'] = $data->getPublishedBy();
        if ($data->isInitialized('publishedByName') && null !== $data->getPublishedByName()) {
            $dataArray['publishedByName'] = $data->getPublishedByName();
        }
        if ($data->isInitialized('republishedFrom') && null !== $data->getRepublishedFrom()) {
            $dataArray['republishedFrom'] = $data->getRepublishedFrom();
        }
        $dataArray['rolloutPercent'] = $data->getRolloutPercent();
        $dataArray['runtimeVersion'] = $data->getRuntimeVersion();
        $dataArray['servedPercent'] = $data->getServedPercent();
        if ($data->isInitialized('sourceCommit') && null !== $data->getSourceCommit()) {
            $dataArray['sourceCommit'] = $data->getSourceCommit();
        }
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AppUpdateResponse::class => false];
    }
}