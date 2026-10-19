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
class RunnerCapabilitiesNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\RunnerCapabilities::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\RunnerCapabilities::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\RunnerCapabilities();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('extra', $data) && $data['extra'] !== null) {
            $values = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['extra'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setExtra($values);
        }
        elseif (\array_key_exists('extra', $data) && $data['extra'] === null) {
            $object->setExtra(null);
        }
        if (\array_key_exists('protocol', $data) && $data['protocol'] !== null) {
            $object->setProtocol($data['protocol']);
        }
        elseif (\array_key_exists('protocol', $data) && $data['protocol'] === null) {
            $object->setProtocol(null);
        }
        if (\array_key_exists('runnerKind', $data) && $data['runnerKind'] !== null) {
            $object->setRunnerKind($data['runnerKind']);
        }
        elseif (\array_key_exists('runnerKind', $data) && $data['runnerKind'] === null) {
            $object->setRunnerKind(null);
        }
        if (\array_key_exists('runtimes', $data) && $data['runtimes'] !== null) {
            $values_1 = [];
            foreach ($data['runtimes'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setRuntimes($values_1);
        }
        elseif (\array_key_exists('runtimes', $data) && $data['runtimes'] === null) {
            $object->setRuntimes(null);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('extra') && null !== $data->getExtra()) {
            $values = [];
            foreach ($data->getExtra() as $key => $value) {
                $values[$key] = $value;
            }
            $dataArray['extra'] = $values;
        }
        $dataArray['protocol'] = $data->getProtocol();
        if ($data->isInitialized('runnerKind') && null !== $data->getRunnerKind()) {
            $dataArray['runnerKind'] = $data->getRunnerKind();
        }
        $values_1 = [];
        foreach ($data->getRuntimes() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['runtimes'] = $values_1;
        if ($data->isInitialized('version') && null !== $data->getVersion()) {
            $dataArray['version'] = $data->getVersion();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\RunnerCapabilities::class => false];
    }
}