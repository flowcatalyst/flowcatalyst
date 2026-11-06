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
class RuntimeTraceNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\RuntimeTrace::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\RuntimeTrace::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\RuntimeTrace();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('hardStop', $data) && $data['hardStop'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['hardStop']);
            if (false === $date) {
                if (is_string($data['hardStop']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['hardStop'])) {
                    try {
                        $date = new \DateTime($data['hardStop']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['hardStop'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['hardStop'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setHardStop($date);
        }
        elseif (\array_key_exists('hardStop', $data) && $data['hardStop'] === null) {
            $object->setHardStop(null);
        }
        if (\array_key_exists('ingestionId', $data) && $data['ingestionId'] !== null) {
            $object->setIngestionId($data['ingestionId']);
        }
        elseif (\array_key_exists('ingestionId', $data) && $data['ingestionId'] === null) {
            $object->setIngestionId(null);
        }
        if (\array_key_exists('sampleEverySecs', $data) && $data['sampleEverySecs'] !== null) {
            $object->setSampleEverySecs($data['sampleEverySecs']);
        }
        elseif (\array_key_exists('sampleEverySecs', $data) && $data['sampleEverySecs'] === null) {
            $object->setSampleEverySecs(null);
        }
        if (\array_key_exists('windowId', $data) && $data['windowId'] !== null) {
            $object->setWindowId($data['windowId']);
        }
        elseif (\array_key_exists('windowId', $data) && $data['windowId'] === null) {
            $object->setWindowId(null);
        }
        if (\array_key_exists('windowSecs', $data) && $data['windowSecs'] !== null) {
            $object->setWindowSecs($data['windowSecs']);
        }
        elseif (\array_key_exists('windowSecs', $data) && $data['windowSecs'] === null) {
            $object->setWindowSecs(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['hardStop'] = $data->getHardStop()->format('Y-m-d\TH:i:sP');
        $dataArray['ingestionId'] = $data->getIngestionId();
        $dataArray['sampleEverySecs'] = $data->getSampleEverySecs();
        $dataArray['windowId'] = $data->getWindowId();
        $dataArray['windowSecs'] = $data->getWindowSecs();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\RuntimeTrace::class => false];
    }
}