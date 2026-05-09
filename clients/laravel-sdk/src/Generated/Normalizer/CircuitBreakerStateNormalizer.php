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
class CircuitBreakerStateNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\CircuitBreakerState::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\CircuitBreakerState::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\CircuitBreakerState();
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
        if (\array_key_exists('lastFailure', $data) && $data['lastFailure'] !== null) {
            $object->setLastFailure($data['lastFailure']);
            unset($data['lastFailure']);
        }
        elseif (\array_key_exists('lastFailure', $data) && $data['lastFailure'] === null) {
            $object->setLastFailure(null);
        }
        if (\array_key_exists('resetAt', $data) && $data['resetAt'] !== null) {
            $object->setResetAt($data['resetAt']);
            unset($data['resetAt']);
        }
        elseif (\array_key_exists('resetAt', $data) && $data['resetAt'] === null) {
            $object->setResetAt(null);
        }
        if (\array_key_exists('state', $data) && $data['state'] !== null) {
            $object->setState($data['state']);
            unset($data['state']);
        }
        elseif (\array_key_exists('state', $data) && $data['state'] === null) {
            $object->setState(null);
        }
        if (\array_key_exists('successCount', $data) && $data['successCount'] !== null) {
            $object->setSuccessCount($data['successCount']);
            unset($data['successCount']);
        }
        elseif (\array_key_exists('successCount', $data) && $data['successCount'] === null) {
            $object->setSuccessCount(null);
        }
        if (\array_key_exists('target', $data) && $data['target'] !== null) {
            $object->setTarget($data['target']);
            unset($data['target']);
        }
        elseif (\array_key_exists('target', $data) && $data['target'] === null) {
            $object->setTarget(null);
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
        if ($data->isInitialized('lastFailure')) {
            $dataArray['lastFailure'] = $data->getLastFailure();
        }
        if ($data->isInitialized('resetAt')) {
            $dataArray['resetAt'] = $data->getResetAt();
        }
        $dataArray['state'] = $data->getState();
        $dataArray['successCount'] = $data->getSuccessCount();
        $dataArray['target'] = $data->getTarget();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\CircuitBreakerState::class => false];
    }
}