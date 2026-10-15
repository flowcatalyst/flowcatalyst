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
class DesiredNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\Desired::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\Desired::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\Desired();
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
        if (\array_key_exists('auth', $data) && $data['auth'] !== null) {
            $object->setAuth($this->denormalizer->denormalize($data['auth'], \FlowCatalyst\Generated\Model\TokenAuth::class, 'json', $context));
        }
        elseif (\array_key_exists('auth', $data) && $data['auth'] === null) {
            $object->setAuth(null);
        }
        if (\array_key_exists('functions', $data) && $data['functions'] !== null) {
            $values = [];
            foreach ($data['functions'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\_Function::class, 'json', $context);
            }
            $object->setFunctions($values);
        }
        elseif (\array_key_exists('functions', $data) && $data['functions'] === null) {
            $object->setFunctions(null);
        }
        if (\array_key_exists('pool', $data) && $data['pool'] !== null) {
            $object->setPool($data['pool']);
        }
        elseif (\array_key_exists('pool', $data) && $data['pool'] === null) {
            $object->setPool(null);
        }
        if (\array_key_exists('revision', $data) && $data['revision'] !== null) {
            $object->setRevision($data['revision']);
        }
        elseif (\array_key_exists('revision', $data) && $data['revision'] === null) {
            $object->setRevision(null);
        }
        if (\array_key_exists('routes', $data) && $data['routes'] !== null) {
            $values_1 = [];
            foreach ($data['routes'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\Route::class, 'json', $context);
            }
            $object->setRoutes($values_1);
        }
        elseif (\array_key_exists('routes', $data) && $data['routes'] === null) {
            $object->setRoutes(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['auth'] = $this->normalizer->normalize($data->getAuth(), 'json', $context);
        $values = [];
        foreach ($data->getFunctions() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['functions'] = $values;
        $dataArray['pool'] = $data->getPool();
        $dataArray['revision'] = $data->getRevision();
        if ($data->isInitialized('routes') && null !== $data->getRoutes()) {
            $values_1 = [];
            foreach ($data->getRoutes() as $value_1) {
                $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
            }
            $dataArray['routes'] = $values_1;
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\Desired::class => false];
    }
}