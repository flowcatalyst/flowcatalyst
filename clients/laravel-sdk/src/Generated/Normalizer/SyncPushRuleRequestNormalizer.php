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
class SyncPushRuleRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\SyncPushRuleRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\SyncPushRuleRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\SyncPushRuleRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('public', $data) && \is_int($data['public'])) {
            $data['public'] = (bool) $data['public'];
        }
        if (\array_key_exists('pattern', $data) && $data['pattern'] !== null) {
            $object->setPattern($data['pattern']);
        }
        elseif (\array_key_exists('pattern', $data) && $data['pattern'] === null) {
            $object->setPattern(null);
        }
        if (\array_key_exists('public', $data) && $data['public'] !== null) {
            $object->setPublic($data['public']);
        }
        elseif (\array_key_exists('public', $data) && $data['public'] === null) {
            $object->setPublic(null);
        }
        if (\array_key_exists('requires', $data) && $data['requires'] !== null) {
            $values = [];
            foreach ($data['requires'] as $value) {
                $values[] = $value;
            }
            $object->setRequires($values);
        }
        elseif (\array_key_exists('requires', $data) && $data['requires'] === null) {
            $object->setRequires(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['pattern'] = $data->getPattern();
        if ($data->isInitialized('public') && null !== $data->getPublic()) {
            $dataArray['public'] = $data->getPublic();
        }
        if ($data->isInitialized('requires') && null !== $data->getRequires()) {
            $values = [];
            foreach ($data->getRequires() as $value) {
                $values[] = $value;
            }
            $dataArray['requires'] = $values;
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\SyncPushRuleRequest::class => false];
    }
}