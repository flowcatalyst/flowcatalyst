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
class TestAnswerNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\TestAnswer::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\TestAnswer::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\TestAnswer();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('excerpt', $data) && $data['excerpt'] !== null) {
            $object->setExcerpt($data['excerpt']);
            unset($data['excerpt']);
        }
        elseif (\array_key_exists('excerpt', $data) && $data['excerpt'] === null) {
            $object->setExcerpt(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
            unset($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('outcome', $data) && $data['outcome'] !== null) {
            $object->setOutcome($data['outcome']);
            unset($data['outcome']);
        }
        elseif (\array_key_exists('outcome', $data) && $data['outcome'] === null) {
            $object->setOutcome(null);
        }
        if (\array_key_exists('records', $data) && $data['records'] !== null) {
            $object->setRecords($data['records']);
            unset($data['records']);
        }
        elseif (\array_key_exists('records', $data) && $data['records'] === null) {
            $object->setRecords(null);
        }
        if (\array_key_exists('steps', $data) && $data['steps'] !== null) {
            $values = [];
            foreach ($data['steps'] as $value) {
                $values[] = $value;
            }
            $object->setSteps($values);
            unset($data['steps']);
        }
        elseif (\array_key_exists('steps', $data) && $data['steps'] === null) {
            $object->setSteps(null);
        }
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_1;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('excerpt') && null !== $data->getExcerpt()) {
            $dataArray['excerpt'] = $data->getExcerpt();
        }
        $dataArray['id'] = $data->getId();
        $dataArray['outcome'] = $data->getOutcome();
        if ($data->isInitialized('records') && null !== $data->getRecords()) {
            $dataArray['records'] = $data->getRecords();
        }
        $values = [];
        foreach ($data->getSteps() as $value) {
            $values[] = $value;
        }
        $dataArray['steps'] = $values;
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\TestAnswer::class => false];
    }
}