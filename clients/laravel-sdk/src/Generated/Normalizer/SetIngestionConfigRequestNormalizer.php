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
class SetIngestionConfigRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\SetIngestionConfigRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\SetIngestionConfigRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\SetIngestionConfigRequest();
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
            unset($data['$schema']);
        }
        if (\array_key_exists('secretFields', $data) && $data['secretFields'] !== null) {
            $values = new \FlowCatalyst\Generated\Runtime\JsonObject();
            foreach ($data['secretFields'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setSecretFields($values);
            unset($data['secretFields']);
        }
        elseif (\array_key_exists('secretFields', $data) && $data['secretFields'] === null) {
            $object->setSecretFields(null);
            unset($data['secretFields']);
        }
        if (\array_key_exists('secrets', $data) && $data['secrets'] !== null) {
            $values_1 = new \FlowCatalyst\Generated\Runtime\JsonObject();
            foreach ($data['secrets'] as $key_1 => $value_1) {
                $values_1[$key_1] = $value_1;
            }
            $object->setSecrets($values_1);
            unset($data['secrets']);
        }
        elseif (\array_key_exists('secrets', $data) && $data['secrets'] === null) {
            $object->setSecrets(null);
            unset($data['secrets']);
        }
        if (\array_key_exists('value', $data) && $data['value'] !== null) {
            $values_2 = new \FlowCatalyst\Generated\Runtime\JsonObject();
            foreach ($data['value'] as $key_2 => $value_2) {
                $values_2[$key_2] = $value_2;
            }
            $object->setValue($values_2);
            unset($data['value']);
        }
        elseif (\array_key_exists('value', $data) && $data['value'] === null) {
            $object->setValue(null);
            unset($data['value']);
        }
        foreach ($data as $key_3 => $value_3) {
            if (preg_match('/.*/', (string) $key_3)) {
                $object[$key_3] = $value_3;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('secretFields') && null !== $data->getSecretFields()) {
            $values = new \FlowCatalyst\Generated\Runtime\JsonObject();
            foreach ($data->getSecretFields() as $key => $value) {
                $values[$key] = $value;
            }
            $dataArray['secretFields'] = $values;
        }
        if ($data->isInitialized('secrets') && null !== $data->getSecrets()) {
            $values_1 = new \FlowCatalyst\Generated\Runtime\JsonObject();
            foreach ($data->getSecrets() as $key_1 => $value_1) {
                $values_1[$key_1] = $value_1;
            }
            $dataArray['secrets'] = $values_1;
        }
        $values_2 = new \FlowCatalyst\Generated\Runtime\JsonObject();
        foreach ($data->getValue() as $key_2 => $value_2) {
            $values_2[$key_2] = $value_2;
        }
        $dataArray['value'] = $values_2;
        foreach ($data->additionalPropertyEntries() as $key_3 => $value_3) {
            if (preg_match('/.*/', (string) $key_3)) {
                $dataArray[$key_3] = $value_3;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\SetIngestionConfigRequest::class => false];
    }
}