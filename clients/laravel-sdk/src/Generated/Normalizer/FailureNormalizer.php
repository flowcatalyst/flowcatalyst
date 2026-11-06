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
class FailureNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\Failure::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\Failure::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\Failure();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('afterRecords', $data) && $data['afterRecords'] !== null) {
            $object->setAfterRecords($data['afterRecords']);
            unset($data['afterRecords']);
        }
        elseif (\array_key_exists('afterRecords', $data) && $data['afterRecords'] === null) {
            $object->setAfterRecords(null);
            unset($data['afterRecords']);
        }
        if (\array_key_exists('at', $data) && $data['at'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['at']);
            if (false === $date) {
                if (is_string($data['at']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['at'])) {
                    try {
                        $date = new \DateTime($data['at']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['at'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['at'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setAt($date);
            unset($data['at']);
        }
        elseif (\array_key_exists('at', $data) && $data['at'] === null) {
            $object->setAt(null);
            unset($data['at']);
        }
        if (\array_key_exists('category', $data) && $data['category'] !== null) {
            $object->setCategory($data['category']);
            unset($data['category']);
        }
        elseif (\array_key_exists('category', $data) && $data['category'] === null) {
            $object->setCategory(null);
            unset($data['category']);
        }
        if (\array_key_exists('excerpt', $data) && $data['excerpt'] !== null) {
            $object->setExcerpt($data['excerpt']);
            unset($data['excerpt']);
        }
        elseif (\array_key_exists('excerpt', $data) && $data['excerpt'] === null) {
            $object->setExcerpt(null);
            unset($data['excerpt']);
        }
        if (\array_key_exists('ingestionId', $data) && $data['ingestionId'] !== null) {
            $object->setIngestionId($data['ingestionId']);
            unset($data['ingestionId']);
        }
        elseif (\array_key_exists('ingestionId', $data) && $data['ingestionId'] === null) {
            $object->setIngestionId(null);
            unset($data['ingestionId']);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
            unset($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
            unset($data['kind']);
        }
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
            unset($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
            unset($data['message']);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
            unset($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
            unset($data['version']);
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
        if ($data->isInitialized('afterRecords') && null !== $data->getAfterRecords()) {
            $dataArray['afterRecords'] = $data->getAfterRecords();
        }
        $dataArray['at'] = $data->getAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('category') && null !== $data->getCategory()) {
            $dataArray['category'] = $data->getCategory();
        }
        if ($data->isInitialized('excerpt') && null !== $data->getExcerpt()) {
            $dataArray['excerpt'] = $data->getExcerpt();
        }
        $dataArray['ingestionId'] = $data->getIngestionId();
        $dataArray['kind'] = $data->getKind();
        $dataArray['message'] = $data->getMessage();
        $dataArray['version'] = $data->getVersion();
        foreach ($data->additionalPropertyEntries() as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\Failure::class => false];
    }
}