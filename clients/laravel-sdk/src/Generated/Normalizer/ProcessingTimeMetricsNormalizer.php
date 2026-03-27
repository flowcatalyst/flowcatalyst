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
class ProcessingTimeMetricsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ProcessingTimeMetrics::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ProcessingTimeMetrics::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ProcessingTimeMetrics();
        if (\array_key_exists('avgMs', $data) && \is_int($data['avgMs'])) {
            $data['avgMs'] = (double) $data['avgMs'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('avgMs', $data) && $data['avgMs'] !== null) {
            $object->setAvgMs($data['avgMs']);
            unset($data['avgMs']);
        }
        elseif (\array_key_exists('avgMs', $data) && $data['avgMs'] === null) {
            $object->setAvgMs(null);
        }
        if (\array_key_exists('maxMs', $data) && $data['maxMs'] !== null) {
            $object->setMaxMs($data['maxMs']);
            unset($data['maxMs']);
        }
        elseif (\array_key_exists('maxMs', $data) && $data['maxMs'] === null) {
            $object->setMaxMs(null);
        }
        if (\array_key_exists('minMs', $data) && $data['minMs'] !== null) {
            $object->setMinMs($data['minMs']);
            unset($data['minMs']);
        }
        elseif (\array_key_exists('minMs', $data) && $data['minMs'] === null) {
            $object->setMinMs(null);
        }
        if (\array_key_exists('p50Ms', $data) && $data['p50Ms'] !== null) {
            $object->setP50Ms($data['p50Ms']);
            unset($data['p50Ms']);
        }
        elseif (\array_key_exists('p50Ms', $data) && $data['p50Ms'] === null) {
            $object->setP50Ms(null);
        }
        if (\array_key_exists('p95Ms', $data) && $data['p95Ms'] !== null) {
            $object->setP95Ms($data['p95Ms']);
            unset($data['p95Ms']);
        }
        elseif (\array_key_exists('p95Ms', $data) && $data['p95Ms'] === null) {
            $object->setP95Ms(null);
        }
        if (\array_key_exists('p99Ms', $data) && $data['p99Ms'] !== null) {
            $object->setP99Ms($data['p99Ms']);
            unset($data['p99Ms']);
        }
        elseif (\array_key_exists('p99Ms', $data) && $data['p99Ms'] === null) {
            $object->setP99Ms(null);
        }
        if (\array_key_exists('sampleCount', $data) && $data['sampleCount'] !== null) {
            $object->setSampleCount($data['sampleCount']);
            unset($data['sampleCount']);
        }
        elseif (\array_key_exists('sampleCount', $data) && $data['sampleCount'] === null) {
            $object->setSampleCount(null);
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
        $dataArray['avgMs'] = $data->getAvgMs();
        $dataArray['maxMs'] = $data->getMaxMs();
        $dataArray['minMs'] = $data->getMinMs();
        $dataArray['p50Ms'] = $data->getP50Ms();
        $dataArray['p95Ms'] = $data->getP95Ms();
        $dataArray['p99Ms'] = $data->getP99Ms();
        $dataArray['sampleCount'] = $data->getSampleCount();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ProcessingTimeMetrics::class => false];
    }
}