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
class IngestionFailureResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionFailureResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionFailureResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionFailureResponse();
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
        if (\array_key_exists('afterRecords', $data) && $data['afterRecords'] !== null) {
            $object->setAfterRecords($data['afterRecords']);
        }
        elseif (\array_key_exists('afterRecords', $data) && $data['afterRecords'] === null) {
            $object->setAfterRecords(null);
        }
        if (\array_key_exists('category', $data) && $data['category'] !== null) {
            $object->setCategory($data['category']);
        }
        elseif (\array_key_exists('category', $data) && $data['category'] === null) {
            $object->setCategory(null);
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
        if (\array_key_exists('dismissedAt', $data) && $data['dismissedAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['dismissedAt']);
            if (false === $date_1) {
                if (is_string($data['dismissedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['dismissedAt'])) {
                    try {
                        $date_1 = new \DateTime($data['dismissedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['dismissedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['dismissedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setDismissedAt($date_1);
        }
        elseif (\array_key_exists('dismissedAt', $data) && $data['dismissedAt'] === null) {
            $object->setDismissedAt(null);
        }
        if (\array_key_exists('dismissedBy', $data) && $data['dismissedBy'] !== null) {
            $object->setDismissedBy($data['dismissedBy']);
        }
        elseif (\array_key_exists('dismissedBy', $data) && $data['dismissedBy'] === null) {
            $object->setDismissedBy(null);
        }
        if (\array_key_exists('excerpt', $data) && $data['excerpt'] !== null) {
            $object->setExcerpt($data['excerpt']);
        }
        elseif (\array_key_exists('excerpt', $data) && $data['excerpt'] === null) {
            $object->setExcerpt(null);
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
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
        }
        if (\array_key_exists('occurredAt', $data) && $data['occurredAt'] !== null) {
            $date_2 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['occurredAt']);
            if (false === $date_2) {
                if (is_string($data['occurredAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['occurredAt'])) {
                    try {
                        $date_2 = new \DateTime($data['occurredAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['occurredAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['occurredAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setOccurredAt($date_2);
        }
        elseif (\array_key_exists('occurredAt', $data) && $data['occurredAt'] === null) {
            $object->setOccurredAt(null);
        }
        if (\array_key_exists('runtimeId', $data) && $data['runtimeId'] !== null) {
            $object->setRuntimeId($data['runtimeId']);
        }
        elseif (\array_key_exists('runtimeId', $data) && $data['runtimeId'] === null) {
            $object->setRuntimeId(null);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['afterRecords'] = $data->getAfterRecords();
        $dataArray['category'] = $data->getCategory();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('dismissedAt') && null !== $data->getDismissedAt()) {
            $dataArray['dismissedAt'] = $data->getDismissedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('dismissedBy') && null !== $data->getDismissedBy()) {
            $dataArray['dismissedBy'] = $data->getDismissedBy();
        }
        if ($data->isInitialized('excerpt') && null !== $data->getExcerpt()) {
            $dataArray['excerpt'] = $data->getExcerpt();
        }
        $dataArray['id'] = $data->getId();
        $dataArray['kind'] = $data->getKind();
        $dataArray['message'] = $data->getMessage();
        $dataArray['occurredAt'] = $data->getOccurredAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('runtimeId') && null !== $data->getRuntimeId()) {
            $dataArray['runtimeId'] = $data->getRuntimeId();
        }
        if ($data->isInitialized('version') && null !== $data->getVersion()) {
            $dataArray['version'] = $data->getVersion();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionFailureResponse::class => false];
    }
}