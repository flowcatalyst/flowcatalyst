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
class PollRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PollRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PollRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PollRequest();
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
        if (\array_key_exists('builtinKinds', $data) && $data['builtinKinds'] !== null) {
            $values = [];
            foreach ($data['builtinKinds'] as $value) {
                $values[] = $value;
            }
            $object->setBuiltinKinds($values);
            unset($data['builtinKinds']);
        }
        elseif (\array_key_exists('builtinKinds', $data) && $data['builtinKinds'] === null) {
            $object->setBuiltinKinds(null);
        }
        if (\array_key_exists('contractVersion', $data) && $data['contractVersion'] !== null) {
            $object->setContractVersion($data['contractVersion']);
            unset($data['contractVersion']);
        }
        elseif (\array_key_exists('contractVersion', $data) && $data['contractVersion'] === null) {
            $object->setContractVersion(null);
        }
        if (\array_key_exists('kinds', $data) && $data['kinds'] !== null) {
            $values_1 = [];
            foreach ($data['kinds'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\RuntimeKind::class, 'json', $context);
            }
            $object->setKinds($values_1);
            unset($data['kinds']);
        }
        elseif (\array_key_exists('kinds', $data) && $data['kinds'] === null) {
            $object->setKinds(null);
        }
        if (\array_key_exists('runtimeId', $data) && $data['runtimeId'] !== null) {
            $object->setRuntimeId($data['runtimeId']);
            unset($data['runtimeId']);
        }
        elseif (\array_key_exists('runtimeId', $data) && $data['runtimeId'] === null) {
            $object->setRuntimeId(null);
        }
        if (\array_key_exists('url', $data) && $data['url'] !== null) {
            $object->setUrl($data['url']);
            unset($data['url']);
        }
        elseif (\array_key_exists('url', $data) && $data['url'] === null) {
            $object->setUrl(null);
        }
        foreach ($data as $key => $value_2) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_2;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('builtinKinds') && null !== $data->getBuiltinKinds()) {
            $values = [];
            foreach ($data->getBuiltinKinds() as $value) {
                $values[] = $value;
            }
            $dataArray['builtinKinds'] = $values;
        }
        $dataArray['contractVersion'] = $data->getContractVersion();
        if ($data->isInitialized('kinds') && null !== $data->getKinds()) {
            $values_1 = [];
            foreach ($data->getKinds() as $value_1) {
                $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
            }
            $dataArray['kinds'] = $values_1;
        }
        $dataArray['runtimeId'] = $data->getRuntimeId();
        if ($data->isInitialized('url') && null !== $data->getUrl()) {
            $dataArray['url'] = $data->getUrl();
        }
        foreach ($data as $key => $value_2) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_2;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PollRequest::class => false];
    }
}