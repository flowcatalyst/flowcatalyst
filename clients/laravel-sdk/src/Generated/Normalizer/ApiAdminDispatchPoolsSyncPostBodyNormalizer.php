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
class ApiAdminDispatchPoolsSyncPostBodyNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody();
        if (\array_key_exists('removeUnlisted', $data) && \is_int($data['removeUnlisted'])) {
            $data['removeUnlisted'] = (bool) $data['removeUnlisted'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
            unset($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('pools', $data) && $data['pools'] !== null) {
            $values = [];
            foreach ($data['pools'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBodyPoolsItem::class, 'json', $context);
            }
            $object->setPools($values);
            unset($data['pools']);
        }
        elseif (\array_key_exists('pools', $data) && $data['pools'] === null) {
            $object->setPools(null);
        }
        if (\array_key_exists('removeUnlisted', $data) && $data['removeUnlisted'] !== null) {
            $object->setRemoveUnlisted($data['removeUnlisted']);
            unset($data['removeUnlisted']);
        }
        elseif (\array_key_exists('removeUnlisted', $data) && $data['removeUnlisted'] === null) {
            $object->setRemoveUnlisted(null);
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
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $values = [];
        foreach ($data->getPools() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['pools'] = $values;
        if ($data->isInitialized('removeUnlisted') && null !== $data->getRemoveUnlisted()) {
            $dataArray['removeUnlisted'] = $data->getRemoveUnlisted();
        }
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody::class => false];
    }
}