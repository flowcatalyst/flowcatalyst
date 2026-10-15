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
class UpdateFunctionRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\UpdateFunctionRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\UpdateFunctionRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\UpdateFunctionRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('clearPool', $data) && \is_int($data['clearPool'])) {
            $data['clearPool'] = (bool) $data['clearPool'];
        }
        if (\array_key_exists('warm', $data) && \is_int($data['warm'])) {
            $data['warm'] = (bool) $data['warm'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('clearPool', $data) && $data['clearPool'] !== null) {
            $object->setClearPool($data['clearPool']);
            unset($data['clearPool']);
        }
        elseif (\array_key_exists('clearPool', $data) && $data['clearPool'] === null) {
            $object->setClearPool(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
            unset($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('limits', $data) && $data['limits'] !== null) {
            $object->setLimits($this->denormalizer->denormalize($data['limits'], \FlowCatalyst\Generated\Model\LimitsDTO::class, 'json', $context));
            unset($data['limits']);
        }
        elseif (\array_key_exists('limits', $data) && $data['limits'] === null) {
            $object->setLimits(null);
        }
        if (\array_key_exists('pool', $data) && $data['pool'] !== null) {
            $object->setPool($data['pool']);
            unset($data['pool']);
        }
        elseif (\array_key_exists('pool', $data) && $data['pool'] === null) {
            $object->setPool(null);
        }
        if (\array_key_exists('warm', $data) && $data['warm'] !== null) {
            $object->setWarm($data['warm']);
            unset($data['warm']);
        }
        elseif (\array_key_exists('warm', $data) && $data['warm'] === null) {
            $object->setWarm(null);
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
        if ($data->isInitialized('clearPool') && null !== $data->getClearPool()) {
            $dataArray['clearPool'] = $data->getClearPool();
        }
        if ($data->isInitialized('description') && null !== $data->getDescription()) {
            $dataArray['description'] = $data->getDescription();
        }
        if ($data->isInitialized('limits') && null !== $data->getLimits()) {
            $dataArray['limits'] = $this->normalizer->normalize($data->getLimits(), 'json', $context);
        }
        if ($data->isInitialized('pool') && null !== $data->getPool()) {
            $dataArray['pool'] = $data->getPool();
        }
        if ($data->isInitialized('warm') && null !== $data->getWarm()) {
            $dataArray['warm'] = $data->getWarm();
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
        return [\FlowCatalyst\Generated\Model\UpdateFunctionRequest::class => false];
    }
}