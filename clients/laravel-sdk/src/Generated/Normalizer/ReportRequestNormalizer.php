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
class ReportRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ReportRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ReportRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\ReportRequest();
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
        if (\array_key_exists('failures', $data) && $data['failures'] !== null) {
            $values = [];
            foreach ($data['failures'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\Failure::class, 'json', $context);
            }
            $object->setFailures($values);
            unset($data['failures']);
        }
        elseif (\array_key_exists('failures', $data) && $data['failures'] === null) {
            $object->setFailures(null);
        }
        if (\array_key_exists('metrics', $data) && $data['metrics'] !== null) {
            $values_1 = [];
            foreach ($data['metrics'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\ExchangeWindow::class, 'json', $context);
            }
            $object->setMetrics($values_1);
            unset($data['metrics']);
        }
        elseif (\array_key_exists('metrics', $data) && $data['metrics'] === null) {
            $object->setMetrics(null);
        }
        if (\array_key_exists('routes', $data) && $data['routes'] !== null) {
            $values_2 = [];
            foreach ($data['routes'] as $value_2) {
                $values_2[] = $this->denormalizer->denormalize($value_2, \FlowCatalyst\Generated\Model\RouteState::class, 'json', $context);
            }
            $object->setRoutes($values_2);
            unset($data['routes']);
        }
        elseif (\array_key_exists('routes', $data) && $data['routes'] === null) {
            $object->setRoutes(null);
        }
        if (\array_key_exists('runtimeId', $data) && $data['runtimeId'] !== null) {
            $object->setRuntimeId($data['runtimeId']);
            unset($data['runtimeId']);
        }
        elseif (\array_key_exists('runtimeId', $data) && $data['runtimeId'] === null) {
            $object->setRuntimeId(null);
        }
        if (\array_key_exists('tests', $data) && $data['tests'] !== null) {
            $values_3 = [];
            foreach ($data['tests'] as $value_3) {
                $values_3[] = $this->denormalizer->denormalize($value_3, \FlowCatalyst\Generated\Model\TestAnswer::class, 'json', $context);
            }
            $object->setTests($values_3);
            unset($data['tests']);
        }
        elseif (\array_key_exists('tests', $data) && $data['tests'] === null) {
            $object->setTests(null);
        }
        foreach ($data as $key => $value_4) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_4;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('failures') && null !== $data->getFailures()) {
            $values = [];
            foreach ($data->getFailures() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['failures'] = $values;
        }
        if ($data->isInitialized('metrics') && null !== $data->getMetrics()) {
            $values_1 = [];
            foreach ($data->getMetrics() as $value_1) {
                $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
            }
            $dataArray['metrics'] = $values_1;
        }
        if ($data->isInitialized('routes') && null !== $data->getRoutes()) {
            $values_2 = [];
            foreach ($data->getRoutes() as $value_2) {
                $values_2[] = $this->normalizer->normalize($value_2, 'json', $context);
            }
            $dataArray['routes'] = $values_2;
        }
        $dataArray['runtimeId'] = $data->getRuntimeId();
        if ($data->isInitialized('tests') && null !== $data->getTests()) {
            $values_3 = [];
            foreach ($data->getTests() as $value_3) {
                $values_3[] = $this->normalizer->normalize($value_3, 'json', $context);
            }
            $dataArray['tests'] = $values_3;
        }
        foreach ($data as $key => $value_4) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_4;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ReportRequest::class => false];
    }
}