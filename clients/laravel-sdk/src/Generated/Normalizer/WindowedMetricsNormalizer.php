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
class WindowedMetricsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\WindowedMetrics::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\WindowedMetrics::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\WindowedMetrics();
        if (\array_key_exists('successRate', $data) && \is_int($data['successRate'])) {
            $data['successRate'] = (double) $data['successRate'];
        }
        if (\array_key_exists('throughputPerSec', $data) && \is_int($data['throughputPerSec'])) {
            $data['throughputPerSec'] = (double) $data['throughputPerSec'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('failureCount', $data) && $data['failureCount'] !== null) {
            $object->setFailureCount($data['failureCount']);
            unset($data['failureCount']);
        }
        elseif (\array_key_exists('failureCount', $data) && $data['failureCount'] === null) {
            $object->setFailureCount(null);
        }
        if (\array_key_exists('processingTime', $data) && $data['processingTime'] !== null) {
            $object->setProcessingTime($this->denormalizer->denormalize($data['processingTime'], \FlowCatalyst\Generated\Model\ProcessingTimeMetrics::class, 'json', $context));
            unset($data['processingTime']);
        }
        elseif (\array_key_exists('processingTime', $data) && $data['processingTime'] === null) {
            $object->setProcessingTime(null);
        }
        if (\array_key_exists('rateLimitedCount', $data) && $data['rateLimitedCount'] !== null) {
            $object->setRateLimitedCount($data['rateLimitedCount']);
            unset($data['rateLimitedCount']);
        }
        elseif (\array_key_exists('rateLimitedCount', $data) && $data['rateLimitedCount'] === null) {
            $object->setRateLimitedCount(null);
        }
        if (\array_key_exists('successCount', $data) && $data['successCount'] !== null) {
            $object->setSuccessCount($data['successCount']);
            unset($data['successCount']);
        }
        elseif (\array_key_exists('successCount', $data) && $data['successCount'] === null) {
            $object->setSuccessCount(null);
        }
        if (\array_key_exists('successRate', $data) && $data['successRate'] !== null) {
            $object->setSuccessRate($data['successRate']);
            unset($data['successRate']);
        }
        elseif (\array_key_exists('successRate', $data) && $data['successRate'] === null) {
            $object->setSuccessRate(null);
        }
        if (\array_key_exists('throughputPerSec', $data) && $data['throughputPerSec'] !== null) {
            $object->setThroughputPerSec($data['throughputPerSec']);
            unset($data['throughputPerSec']);
        }
        elseif (\array_key_exists('throughputPerSec', $data) && $data['throughputPerSec'] === null) {
            $object->setThroughputPerSec(null);
        }
        if (\array_key_exists('windowDurationSecs', $data) && $data['windowDurationSecs'] !== null) {
            $object->setWindowDurationSecs($data['windowDurationSecs']);
            unset($data['windowDurationSecs']);
        }
        elseif (\array_key_exists('windowDurationSecs', $data) && $data['windowDurationSecs'] === null) {
            $object->setWindowDurationSecs(null);
        }
        if (\array_key_exists('windowStart', $data) && $data['windowStart'] !== null) {
            $object->setWindowStart(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['windowStart']));
            unset($data['windowStart']);
        }
        elseif (\array_key_exists('windowStart', $data) && $data['windowStart'] === null) {
            $object->setWindowStart(null);
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
        $dataArray['failureCount'] = $data->getFailureCount();
        $dataArray['processingTime'] = $this->normalizer->normalize($data->getProcessingTime(), 'json', $context);
        $dataArray['rateLimitedCount'] = $data->getRateLimitedCount();
        $dataArray['successCount'] = $data->getSuccessCount();
        $dataArray['successRate'] = $data->getSuccessRate();
        $dataArray['throughputPerSec'] = $data->getThroughputPerSec();
        $dataArray['windowDurationSecs'] = $data->getWindowDurationSecs();
        $dataArray['windowStart'] = $data->getWindowStart()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\WindowedMetrics::class => false];
    }
}