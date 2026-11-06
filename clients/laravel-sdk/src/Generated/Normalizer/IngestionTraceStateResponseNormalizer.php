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
class IngestionTraceStateResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionTraceStateResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionTraceStateResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionTraceStateResponse();
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
        if (\array_key_exists('enabledAt', $data) && $data['enabledAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['enabledAt']);
            if (false === $date) {
                if (is_string($data['enabledAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['enabledAt'])) {
                    try {
                        $date = new \DateTime($data['enabledAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['enabledAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['enabledAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setEnabledAt($date);
        }
        elseif (\array_key_exists('enabledAt', $data) && $data['enabledAt'] === null) {
            $object->setEnabledAt(null);
        }
        if (\array_key_exists('enabledBy', $data) && $data['enabledBy'] !== null) {
            $object->setEnabledBy($data['enabledBy']);
        }
        elseif (\array_key_exists('enabledBy', $data) && $data['enabledBy'] === null) {
            $object->setEnabledBy(null);
        }
        if (\array_key_exists('hardStop', $data) && $data['hardStop'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['hardStop']);
            if (false === $date_1) {
                if (is_string($data['hardStop']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['hardStop'])) {
                    try {
                        $date_1 = new \DateTime($data['hardStop']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['hardStop'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['hardStop'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setHardStop($date_1);
        }
        elseif (\array_key_exists('hardStop', $data) && $data['hardStop'] === null) {
            $object->setHardStop(null);
        }
        if (\array_key_exists('live', $data) && $data['live'] !== null) {
            $object->setLive($data['live']);
        }
        elseif (\array_key_exists('live', $data) && $data['live'] === null) {
            $object->setLive(null);
        }
        if (\array_key_exists('maxWindowSecs', $data) && $data['maxWindowSecs'] !== null) {
            $object->setMaxWindowSecs($data['maxWindowSecs']);
        }
        elseif (\array_key_exists('maxWindowSecs', $data) && $data['maxWindowSecs'] === null) {
            $object->setMaxWindowSecs(null);
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
        $dataArray['enabledAt'] = $data->getEnabledAt()?->format('Y-m-d\TH:i:sP');
        $dataArray['enabledBy'] = $data->getEnabledBy();
        $dataArray['hardStop'] = $data->getHardStop()?->format('Y-m-d\TH:i:sP');
        $dataArray['live'] = $data->getLive();
        $dataArray['maxWindowSecs'] = $data->getMaxWindowSecs();
        $dataArray['sampleEverySecs'] = $data->getSampleEverySecs();
        $dataArray['windowId'] = $data->getWindowId();
        $dataArray['windowSecs'] = $data->getWindowSecs();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionTraceStateResponse::class => false];
    }
}