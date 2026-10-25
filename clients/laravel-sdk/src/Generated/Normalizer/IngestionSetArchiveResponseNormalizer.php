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
class IngestionSetArchiveResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionSetArchiveResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionSetArchiveResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionSetArchiveResponse();
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
        if (\array_key_exists('appliedAt', $data) && $data['appliedAt'] !== null) {
            $object->setAppliedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['appliedAt']));
        }
        elseif (\array_key_exists('appliedAt', $data) && $data['appliedAt'] === null) {
            $object->setAppliedAt(null);
        }
        if (\array_key_exists('appliedBy', $data) && $data['appliedBy'] !== null) {
            $object->setAppliedBy($data['appliedBy']);
        }
        elseif (\array_key_exists('appliedBy', $data) && $data['appliedBy'] === null) {
            $object->setAppliedBy(null);
        }
        if (\array_key_exists('applyId', $data) && $data['applyId'] !== null) {
            $object->setApplyId($data['applyId']);
        }
        elseif (\array_key_exists('applyId', $data) && $data['applyId'] === null) {
            $object->setApplyId(null);
        }
        if (\array_key_exists('files', $data) && $data['files'] !== null) {
            $values = [];
            foreach ($data['files'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\SetFile::class, 'json', $context);
            }
            $object->setFiles($values);
        }
        elseif (\array_key_exists('files', $data) && $data['files'] === null) {
            $object->setFiles(null);
        }
        if (\array_key_exists('goingBackTo', $data) && $data['goingBackTo'] !== null) {
            $object->setGoingBackTo($data['goingBackTo']);
        }
        elseif (\array_key_exists('goingBackTo', $data) && $data['goingBackTo'] === null) {
            $object->setGoingBackTo(null);
        }
        if (\array_key_exists('items', $data) && $data['items'] !== null) {
            $values_1 = [];
            foreach ($data['items'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\IngestionSetItem::class, 'json', $context);
            }
            $object->setItems($values_1);
        }
        elseif (\array_key_exists('items', $data) && $data['items'] === null) {
            $object->setItems(null);
        }
        if (\array_key_exists('set', $data) && $data['set'] !== null) {
            $object->setSet($data['set']);
        }
        elseif (\array_key_exists('set', $data) && $data['set'] === null) {
            $object->setSet(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['appliedAt'] = $data->getAppliedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['appliedBy'] = $data->getAppliedBy();
        $dataArray['applyId'] = $data->getApplyId();
        $values = [];
        foreach ($data->getFiles() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['files'] = $values;
        if ($data->isInitialized('goingBackTo') && null !== $data->getGoingBackTo()) {
            $dataArray['goingBackTo'] = $data->getGoingBackTo();
        }
        $values_1 = [];
        foreach ($data->getItems() as $value_1) {
            $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
        }
        $dataArray['items'] = $values_1;
        $dataArray['set'] = $data->getSet();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionSetArchiveResponse::class => false];
    }
}