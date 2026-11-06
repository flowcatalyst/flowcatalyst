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
class PollResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PollResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PollResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PollResponse();
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
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('asOf', $data) && $data['asOf'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['asOf']);
            if (false === $date) {
                if (is_string($data['asOf']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['asOf'])) {
                    try {
                        $date = new \DateTime($data['asOf']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['asOf'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['asOf'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setAsOf($date);
        }
        elseif (\array_key_exists('asOf', $data) && $data['asOf'] === null) {
            $object->setAsOf(null);
        }
        if (\array_key_exists('ingestions', $data) && $data['ingestions'] !== null) {
            $values = [];
            foreach ($data['ingestions'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\RuntimeIngestion::class, 'json', $context);
            }
            $object->setIngestions($values);
        }
        elseif (\array_key_exists('ingestions', $data) && $data['ingestions'] === null) {
            $object->setIngestions(null);
        }
        if (\array_key_exists('leaseFor', $data) && $data['leaseFor'] !== null) {
            $object->setLeaseFor($data['leaseFor']);
        }
        elseif (\array_key_exists('leaseFor', $data) && $data['leaseFor'] === null) {
            $object->setLeaseFor(null);
        }
        if (\array_key_exists('tests', $data) && $data['tests'] !== null) {
            $values_1 = [];
            foreach ($data['tests'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\RuntimeTest::class, 'json', $context);
            }
            $object->setTests($values_1);
        }
        elseif (\array_key_exists('tests', $data) && $data['tests'] === null) {
            $object->setTests(null);
        }
        if (\array_key_exists('traces', $data) && $data['traces'] !== null) {
            $values_2 = [];
            foreach ($data['traces'] as $value_2) {
                $values_2[] = $this->denormalizer->denormalize($value_2, \FlowCatalyst\Generated\Model\RuntimeTrace::class, 'json', $context);
            }
            $object->setTraces($values_2);
        }
        elseif (\array_key_exists('traces', $data) && $data['traces'] === null) {
            $object->setTraces(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['asOf'] = $data->getAsOf()->format('Y-m-d\TH:i:sP');
        $values = [];
        foreach ($data->getIngestions() as $value) {
            $values[] = $value === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value, 'json', $context));
        }
        $dataArray['ingestions'] = $values;
        $dataArray['leaseFor'] = $data->getLeaseFor();
        $values_1 = [];
        foreach ($data->getTests() as $value_1) {
            $values_1[] = $value_1 === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value_1, 'json', $context));
        }
        $dataArray['tests'] = $values_1;
        if ($data->isInitialized('traces') && null !== $data->getTraces()) {
            $values_2 = [];
            foreach ($data->getTraces() as $value_2) {
                $values_2[] = $value_2 === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value_2, 'json', $context));
            }
            $dataArray['traces'] = $values_2;
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PollResponse::class => false];
    }
}