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
class DispatchRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\DispatchRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\DispatchRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\DispatchRequest();
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
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('dispatch', $data) && $data['dispatch'] !== null) {
            $object->setDispatch($this->denormalizer->denormalize($data['dispatch'], \FlowCatalyst\Generated\Model\Dispatch::class, 'json', $context));
            unset($data['dispatch']);
        }
        elseif (\array_key_exists('dispatch', $data) && $data['dispatch'] === null) {
            $object->setDispatch(null);
        }
        if (\array_key_exists('functionId', $data) && $data['functionId'] !== null) {
            $object->setFunctionId($data['functionId']);
            unset($data['functionId']);
        }
        elseif (\array_key_exists('functionId', $data) && $data['functionId'] === null) {
            $object->setFunctionId(null);
        }
        if (\array_key_exists('invocationId', $data) && $data['invocationId'] !== null) {
            $object->setInvocationId($data['invocationId']);
            unset($data['invocationId']);
        }
        elseif (\array_key_exists('invocationId', $data) && $data['invocationId'] === null) {
            $object->setInvocationId(null);
        }
        if (\array_key_exists('payload', $data) && $data['payload'] !== null) {
            $object->setPayload($data['payload']);
            unset($data['payload']);
        }
        elseif (\array_key_exists('payload', $data) && $data['payload'] === null) {
            $object->setPayload(null);
        }
        if (\array_key_exists('requestCorrelationId', $data) && $data['requestCorrelationId'] !== null) {
            $object->setRequestCorrelationId($data['requestCorrelationId']);
            unset($data['requestCorrelationId']);
        }
        elseif (\array_key_exists('requestCorrelationId', $data) && $data['requestCorrelationId'] === null) {
            $object->setRequestCorrelationId(null);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
            unset($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
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
        $dataArray['dispatch'] = $this->normalizer->normalize($data->getDispatch(), 'json', $context);
        $dataArray['functionId'] = $data->getFunctionId();
        if ($data->isInitialized('invocationId') && null !== $data->getInvocationId()) {
            $dataArray['invocationId'] = $data->getInvocationId();
        }
        $dataArray['payload'] = $data->getPayload();
        if ($data->isInitialized('requestCorrelationId') && null !== $data->getRequestCorrelationId()) {
            $dataArray['requestCorrelationId'] = $data->getRequestCorrelationId();
        }
        $dataArray['version'] = $data->getVersion();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\DispatchRequest::class => false];
    }
}