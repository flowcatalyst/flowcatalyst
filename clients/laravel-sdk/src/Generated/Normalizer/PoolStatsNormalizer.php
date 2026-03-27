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
class PoolStatsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PoolStats::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PoolStats::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\PoolStats();
        if (\array_key_exists('is_rate_limited', $data) && \is_int($data['is_rate_limited'])) {
            $data['is_rate_limited'] = (bool) $data['is_rate_limited'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('active_workers', $data) && $data['active_workers'] !== null) {
            $object->setActiveWorkers($data['active_workers']);
            unset($data['active_workers']);
        }
        elseif (\array_key_exists('active_workers', $data) && $data['active_workers'] === null) {
            $object->setActiveWorkers(null);
        }
        if (\array_key_exists('concurrency', $data) && $data['concurrency'] !== null) {
            $object->setConcurrency($data['concurrency']);
            unset($data['concurrency']);
        }
        elseif (\array_key_exists('concurrency', $data) && $data['concurrency'] === null) {
            $object->setConcurrency(null);
        }
        if (\array_key_exists('is_rate_limited', $data) && $data['is_rate_limited'] !== null) {
            $object->setIsRateLimited($data['is_rate_limited']);
            unset($data['is_rate_limited']);
        }
        elseif (\array_key_exists('is_rate_limited', $data) && $data['is_rate_limited'] === null) {
            $object->setIsRateLimited(null);
        }
        if (\array_key_exists('message_group_count', $data) && $data['message_group_count'] !== null) {
            $object->setMessageGroupCount($data['message_group_count']);
            unset($data['message_group_count']);
        }
        elseif (\array_key_exists('message_group_count', $data) && $data['message_group_count'] === null) {
            $object->setMessageGroupCount(null);
        }
        if (\array_key_exists('metrics', $data) && $data['metrics'] !== null) {
            $object->setMetrics($data['metrics']);
            unset($data['metrics']);
        }
        elseif (\array_key_exists('metrics', $data) && $data['metrics'] === null) {
            $object->setMetrics(null);
        }
        if (\array_key_exists('pool_code', $data) && $data['pool_code'] !== null) {
            $object->setPoolCode($data['pool_code']);
            unset($data['pool_code']);
        }
        elseif (\array_key_exists('pool_code', $data) && $data['pool_code'] === null) {
            $object->setPoolCode(null);
        }
        if (\array_key_exists('queue_capacity', $data) && $data['queue_capacity'] !== null) {
            $object->setQueueCapacity($data['queue_capacity']);
            unset($data['queue_capacity']);
        }
        elseif (\array_key_exists('queue_capacity', $data) && $data['queue_capacity'] === null) {
            $object->setQueueCapacity(null);
        }
        if (\array_key_exists('queue_size', $data) && $data['queue_size'] !== null) {
            $object->setQueueSize($data['queue_size']);
            unset($data['queue_size']);
        }
        elseif (\array_key_exists('queue_size', $data) && $data['queue_size'] === null) {
            $object->setQueueSize(null);
        }
        if (\array_key_exists('rate_limit_per_minute', $data) && $data['rate_limit_per_minute'] !== null) {
            $object->setRateLimitPerMinute($data['rate_limit_per_minute']);
            unset($data['rate_limit_per_minute']);
        }
        elseif (\array_key_exists('rate_limit_per_minute', $data) && $data['rate_limit_per_minute'] === null) {
            $object->setRateLimitPerMinute(null);
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
        $dataArray['active_workers'] = $data->getActiveWorkers();
        $dataArray['concurrency'] = $data->getConcurrency();
        $dataArray['is_rate_limited'] = $data->getIsRateLimited();
        $dataArray['message_group_count'] = $data->getMessageGroupCount();
        if ($data->isInitialized('metrics') && null !== $data->getMetrics()) {
            $dataArray['metrics'] = $data->getMetrics();
        }
        $dataArray['pool_code'] = $data->getPoolCode();
        $dataArray['queue_capacity'] = $data->getQueueCapacity();
        $dataArray['queue_size'] = $data->getQueueSize();
        if ($data->isInitialized('rateLimitPerMinute')) {
            $dataArray['rate_limit_per_minute'] = $data->getRateLimitPerMinute();
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PoolStats::class => false];
    }
}