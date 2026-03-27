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
class EnhancedPoolMetricsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\EnhancedPoolMetrics::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\EnhancedPoolMetrics::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\EnhancedPoolMetrics();
        if (\array_key_exists('successRate', $data) && \is_int($data['successRate'])) {
            $data['successRate'] = (double) $data['successRate'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('last30Min', $data) && $data['last30Min'] !== null) {
            $object->setLast30Min($this->denormalizer->denormalize($data['last30Min'], \FlowCatalyst\Generated\Model\WindowedMetrics::class, 'json', $context));
            unset($data['last30Min']);
        }
        elseif (\array_key_exists('last30Min', $data) && $data['last30Min'] === null) {
            $object->setLast30Min(null);
        }
        if (\array_key_exists('last5Min', $data) && $data['last5Min'] !== null) {
            $object->setLast5Min($this->denormalizer->denormalize($data['last5Min'], \FlowCatalyst\Generated\Model\WindowedMetrics::class, 'json', $context));
            unset($data['last5Min']);
        }
        elseif (\array_key_exists('last5Min', $data) && $data['last5Min'] === null) {
            $object->setLast5Min(null);
        }
        if (\array_key_exists('processingTime', $data) && $data['processingTime'] !== null) {
            $object->setProcessingTime($this->denormalizer->denormalize($data['processingTime'], \FlowCatalyst\Generated\Model\ProcessingTimeMetrics::class, 'json', $context));
            unset($data['processingTime']);
        }
        elseif (\array_key_exists('processingTime', $data) && $data['processingTime'] === null) {
            $object->setProcessingTime(null);
        }
        if (\array_key_exists('successRate', $data) && $data['successRate'] !== null) {
            $object->setSuccessRate($data['successRate']);
            unset($data['successRate']);
        }
        elseif (\array_key_exists('successRate', $data) && $data['successRate'] === null) {
            $object->setSuccessRate(null);
        }
        if (\array_key_exists('totalFailure', $data) && $data['totalFailure'] !== null) {
            $object->setTotalFailure($data['totalFailure']);
            unset($data['totalFailure']);
        }
        elseif (\array_key_exists('totalFailure', $data) && $data['totalFailure'] === null) {
            $object->setTotalFailure(null);
        }
        if (\array_key_exists('totalRateLimited', $data) && $data['totalRateLimited'] !== null) {
            $object->setTotalRateLimited($data['totalRateLimited']);
            unset($data['totalRateLimited']);
        }
        elseif (\array_key_exists('totalRateLimited', $data) && $data['totalRateLimited'] === null) {
            $object->setTotalRateLimited(null);
        }
        if (\array_key_exists('totalSuccess', $data) && $data['totalSuccess'] !== null) {
            $object->setTotalSuccess($data['totalSuccess']);
            unset($data['totalSuccess']);
        }
        elseif (\array_key_exists('totalSuccess', $data) && $data['totalSuccess'] === null) {
            $object->setTotalSuccess(null);
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
        $dataArray['last30Min'] = $this->normalizer->normalize($data->getLast30Min(), 'json', $context);
        $dataArray['last5Min'] = $this->normalizer->normalize($data->getLast5Min(), 'json', $context);
        $dataArray['processingTime'] = $this->normalizer->normalize($data->getProcessingTime(), 'json', $context);
        $dataArray['successRate'] = $data->getSuccessRate();
        $dataArray['totalFailure'] = $data->getTotalFailure();
        $dataArray['totalRateLimited'] = $data->getTotalRateLimited();
        $dataArray['totalSuccess'] = $data->getTotalSuccess();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\EnhancedPoolMetrics::class => false];
    }
}