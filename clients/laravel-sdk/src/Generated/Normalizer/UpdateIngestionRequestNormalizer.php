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
class UpdateIngestionRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\UpdateIngestionRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\UpdateIngestionRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\UpdateIngestionRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('keepFailedPayloads', $data) && \is_int($data['keepFailedPayloads'])) {
            $data['keepFailedPayloads'] = (bool) $data['keepFailedPayloads'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('expectedVersion', $data) && $data['expectedVersion'] !== null) {
            $object->setExpectedVersion($data['expectedVersion']);
            unset($data['expectedVersion']);
        }
        elseif (\array_key_exists('expectedVersion', $data) && $data['expectedVersion'] === null) {
            $object->setExpectedVersion(null);
        }
        if (\array_key_exists('keepFailedPayloads', $data) && $data['keepFailedPayloads'] !== null) {
            $object->setKeepFailedPayloads($data['keepFailedPayloads']);
            unset($data['keepFailedPayloads']);
        }
        elseif (\array_key_exists('keepFailedPayloads', $data) && $data['keepFailedPayloads'] === null) {
            $object->setKeepFailedPayloads(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
            unset($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('note', $data) && $data['note'] !== null) {
            $object->setNote($data['note']);
            unset($data['note']);
        }
        elseif (\array_key_exists('note', $data) && $data['note'] === null) {
            $object->setNote(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
            unset($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('split', $data) && $data['split'] !== null) {
            $object->setSplit($data['split']);
            unset($data['split']);
        }
        elseif (\array_key_exists('split', $data) && $data['split'] === null) {
            $object->setSplit(null);
        }
        if (\array_key_exists('target', $data) && $data['target'] !== null) {
            $object->setTarget($this->denormalizer->denormalize($data['target'], \FlowCatalyst\Generated\Model\Target::class, 'json', $context));
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
        $dataArray['expectedVersion'] = $data->getExpectedVersion();
        if ($data->isInitialized('keepFailedPayloads') && null !== $data->getKeepFailedPayloads()) {
            $dataArray['keepFailedPayloads'] = $data->getKeepFailedPayloads();
        }
        $dataArray['name'] = $data->getName();
        if ($data->isInitialized('note') && null !== $data->getNote()) {
            $dataArray['note'] = $data->getNote();
        }
        $dataArray['source'] = $data->getSource();
        $dataArray['split'] = $data->getSplit();
        $dataArray['target'] = $this->normalizer->normalize($data->getTarget(), 'json', $context);
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\UpdateIngestionRequest::class => false];
    }
}