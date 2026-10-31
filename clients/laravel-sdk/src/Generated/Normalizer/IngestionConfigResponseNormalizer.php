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
class IngestionConfigResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionConfigResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionConfigResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionConfigResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('schemaChecked', $data) && \is_int($data['schemaChecked'])) {
            $data['schemaChecked'] = (bool) $data['schemaChecked'];
        }
        if (\array_key_exists('valueHidden', $data) && \is_int($data['valueHidden'])) {
            $data['valueHidden'] = (bool) $data['valueHidden'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('key', $data) && $data['key'] !== null) {
            $object->setKey($data['key']);
        }
        elseif (\array_key_exists('key', $data) && $data['key'] === null) {
            $object->setKey(null);
        }
        if (\array_key_exists('problems', $data) && $data['problems'] !== null) {
            $values = [];
            foreach ($data['problems'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\FieldError::class, 'json', $context);
            }
            $object->setProblems($values);
        }
        elseif (\array_key_exists('problems', $data) && $data['problems'] === null) {
            $object->setProblems(null);
        }
        if (\array_key_exists('schemaChecked', $data) && $data['schemaChecked'] !== null) {
            $object->setSchemaChecked($data['schemaChecked']);
        }
        elseif (\array_key_exists('schemaChecked', $data) && $data['schemaChecked'] === null) {
            $object->setSchemaChecked(null);
        }
        if (\array_key_exists('secrets', $data) && $data['secrets'] !== null) {
            $values_1 = [];
            foreach ($data['secrets'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\IngestionConfigSecret::class, 'json', $context);
            }
            $object->setSecrets($values_1);
        }
        elseif (\array_key_exists('secrets', $data) && $data['secrets'] === null) {
            $object->setSecrets(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        if (\array_key_exists('value', $data) && $data['value'] !== null) {
            $values_2 = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['value'] as $key => $value_2) {
                $values_2[$key] = $value_2;
            }
            $object->setValue($values_2);
        }
        elseif (\array_key_exists('value', $data) && $data['value'] === null) {
            $object->setValue(null);
        }
        if (\array_key_exists('valueHidden', $data) && $data['valueHidden'] !== null) {
            $object->setValueHidden($data['valueHidden']);
        }
        elseif (\array_key_exists('valueHidden', $data) && $data['valueHidden'] === null) {
            $object->setValueHidden(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('key') && null !== $data->getKey()) {
            $dataArray['key'] = $data->getKey();
        }
        if ($data->isInitialized('problems') && null !== $data->getProblems()) {
            $values = [];
            foreach ($data->getProblems() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['problems'] = $values;
        }
        $dataArray['schemaChecked'] = $data->getSchemaChecked();
        if ($data->isInitialized('secrets') && null !== $data->getSecrets()) {
            $values_1 = [];
            foreach ($data->getSecrets() as $value_1) {
                $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
            }
            $dataArray['secrets'] = $values_1;
        }
        $dataArray['status'] = $data->getStatus();
        if ($data->isInitialized('updatedAt') && null !== $data->getUpdatedAt()) {
            $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('value') && null !== $data->getValue()) {
            $values_2 = [];
            foreach ($data->getValue() as $key => $value_2) {
                $values_2[$key] = $value_2;
            }
            $dataArray['value'] = $values_2;
        }
        if ($data->isInitialized('valueHidden') && null !== $data->getValueHidden()) {
            $dataArray['valueHidden'] = $data->getValueHidden();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionConfigResponse::class => false];
    }
}