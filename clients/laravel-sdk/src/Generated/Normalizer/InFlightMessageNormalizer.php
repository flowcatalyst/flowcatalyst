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
class InFlightMessageNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\InFlightMessage::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\InFlightMessage::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\InFlightMessage();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('attempt', $data) && $data['attempt'] !== null) {
            $object->setAttempt($data['attempt']);
            unset($data['attempt']);
        }
        elseif (\array_key_exists('attempt', $data) && $data['attempt'] === null) {
            $object->setAttempt(null);
        }
        if (\array_key_exists('elapsedMs', $data) && $data['elapsedMs'] !== null) {
            $object->setElapsedMs($data['elapsedMs']);
            unset($data['elapsedMs']);
        }
        elseif (\array_key_exists('elapsedMs', $data) && $data['elapsedMs'] === null) {
            $object->setElapsedMs(null);
        }
        if (\array_key_exists('eventId', $data) && $data['eventId'] !== null) {
            $object->setEventId($data['eventId']);
            unset($data['eventId']);
        }
        elseif (\array_key_exists('eventId', $data) && $data['eventId'] === null) {
            $object->setEventId(null);
        }
        if (\array_key_exists('jobId', $data) && $data['jobId'] !== null) {
            $object->setJobId($data['jobId']);
            unset($data['jobId']);
        }
        elseif (\array_key_exists('jobId', $data) && $data['jobId'] === null) {
            $object->setJobId(null);
        }
        if (\array_key_exists('messageGroup', $data) && $data['messageGroup'] !== null) {
            $object->setMessageGroup($data['messageGroup']);
            unset($data['messageGroup']);
        }
        elseif (\array_key_exists('messageGroup', $data) && $data['messageGroup'] === null) {
            $object->setMessageGroup(null);
        }
        if (\array_key_exists('poolId', $data) && $data['poolId'] !== null) {
            $object->setPoolId($data['poolId']);
            unset($data['poolId']);
        }
        elseif (\array_key_exists('poolId', $data) && $data['poolId'] === null) {
            $object->setPoolId(null);
        }
        if (\array_key_exists('startedAt', $data) && $data['startedAt'] !== null) {
            $object->setStartedAt($data['startedAt']);
            unset($data['startedAt']);
        }
        elseif (\array_key_exists('startedAt', $data) && $data['startedAt'] === null) {
            $object->setStartedAt(null);
        }
        if (\array_key_exists('targetUrl', $data) && $data['targetUrl'] !== null) {
            $object->setTargetUrl($data['targetUrl']);
            unset($data['targetUrl']);
        }
        elseif (\array_key_exists('targetUrl', $data) && $data['targetUrl'] === null) {
            $object->setTargetUrl(null);
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
        $dataArray['attempt'] = $data->getAttempt();
        $dataArray['elapsedMs'] = $data->getElapsedMs();
        if ($data->isInitialized('eventId')) {
            $dataArray['eventId'] = $data->getEventId();
        }
        $dataArray['jobId'] = $data->getJobId();
        if ($data->isInitialized('messageGroup')) {
            $dataArray['messageGroup'] = $data->getMessageGroup();
        }
        if ($data->isInitialized('poolId')) {
            $dataArray['poolId'] = $data->getPoolId();
        }
        $dataArray['startedAt'] = $data->getStartedAt();
        $dataArray['targetUrl'] = $data->getTargetUrl();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\InFlightMessage::class => false];
    }
}