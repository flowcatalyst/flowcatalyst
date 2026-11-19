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
class AppBinaryResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AppBinaryResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AppBinaryResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\AppBinaryResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('newest', $data) && \is_int($data['newest'])) {
            $data['newest'] = (bool) $data['newest'];
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
        if (\array_key_exists('facts', $data) && $data['facts'] !== null) {
            $object->setFacts($this->denormalizer->denormalize($data['facts'], \FlowCatalyst\Generated\Model\AppBinaryFactsResponse::class, 'json', $context));
        }
        elseif (\array_key_exists('facts', $data) && $data['facts'] === null) {
            $object->setFacts(null);
        }
        if (\array_key_exists('fileName', $data) && $data['fileName'] !== null) {
            $object->setFileName($data['fileName']);
        }
        elseif (\array_key_exists('fileName', $data) && $data['fileName'] === null) {
            $object->setFileName(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
        }
        if (\array_key_exists('newest', $data) && $data['newest'] !== null) {
            $object->setNewest($data['newest']);
        }
        elseif (\array_key_exists('newest', $data) && $data['newest'] === null) {
            $object->setNewest(null);
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
        if (\array_key_exists('sha256', $data) && $data['sha256'] !== null) {
            $object->setSha256($data['sha256']);
        }
        elseif (\array_key_exists('sha256', $data) && $data['sha256'] === null) {
            $object->setSha256(null);
        }
        if (\array_key_exists('sizeBytes', $data) && $data['sizeBytes'] !== null) {
            $object->setSizeBytes($data['sizeBytes']);
        }
        elseif (\array_key_exists('sizeBytes', $data) && $data['sizeBytes'] === null) {
            $object->setSizeBytes(null);
        }
        if (\array_key_exists('sourceCommit', $data) && $data['sourceCommit'] !== null) {
            $object->setSourceCommit($data['sourceCommit']);
        }
        elseif (\array_key_exists('sourceCommit', $data) && $data['sourceCommit'] === null) {
            $object->setSourceCommit(null);
        }
        if (\array_key_exists('variant', $data) && $data['variant'] !== null) {
            $object->setVariant($data['variant']);
        }
        elseif (\array_key_exists('variant', $data) && $data['variant'] === null) {
            $object->setVariant(null);
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
        $dataArray['facts'] = $data->getFacts() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getFacts(), 'json', $context));
        $dataArray['fileName'] = $data->getFileName();
        $dataArray['id'] = $data->getId();
        if ($data->isInitialized('message') && null !== $data->getMessage()) {
            $dataArray['message'] = $data->getMessage();
        }
        $dataArray['newest'] = $data->getNewest();
        $dataArray['publishedBy'] = $data->getPublishedBy();
        if ($data->isInitialized('publishedByName') && null !== $data->getPublishedByName()) {
            $dataArray['publishedByName'] = $data->getPublishedByName();
        }
        $dataArray['sha256'] = $data->getSha256();
        $dataArray['sizeBytes'] = $data->getSizeBytes();
        if ($data->isInitialized('sourceCommit') && null !== $data->getSourceCommit()) {
            $dataArray['sourceCommit'] = $data->getSourceCommit();
        }
        $dataArray['variant'] = $data->getVariant();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AppBinaryResponse::class => false];
    }
}