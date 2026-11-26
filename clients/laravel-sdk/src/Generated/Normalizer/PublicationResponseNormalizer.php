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
class PublicationResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PublicationResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PublicationResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PublicationResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('available', $data) && \is_int($data['available'])) {
            $data['available'] = (bool) $data['available'];
        }
        if (\array_key_exists('enabled', $data) && \is_int($data['enabled'])) {
            $data['enabled'] = (bool) $data['enabled'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('available', $data) && $data['available'] !== null) {
            $object->setAvailable($data['available']);
        }
        elseif (\array_key_exists('available', $data) && $data['available'] === null) {
            $object->setAvailable(null);
        }
        if (\array_key_exists('basePath', $data) && $data['basePath'] !== null) {
            $object->setBasePath($data['basePath']);
        }
        elseif (\array_key_exists('basePath', $data) && $data['basePath'] === null) {
            $object->setBasePath(null);
        }
        if (\array_key_exists('enabled', $data) && $data['enabled'] !== null) {
            $object->setEnabled($data['enabled']);
        }
        elseif (\array_key_exists('enabled', $data) && $data['enabled'] === null) {
            $object->setEnabled(null);
        }
        if (\array_key_exists('enabledAt', $data) && $data['enabledAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['enabledAt']);
            if (false === $date) {
                if (is_string($data['enabledAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['enabledAt'])) {
                    try {
                        $date = new \DateTime($data['enabledAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['enabledAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['enabledAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setEnabledAt($date);
        }
        elseif (\array_key_exists('enabledAt', $data) && $data['enabledAt'] === null) {
            $object->setEnabledAt(null);
        }
        if (\array_key_exists('enabledBy', $data) && $data['enabledBy'] !== null) {
            $object->setEnabledBy($data['enabledBy']);
        }
        elseif (\array_key_exists('enabledBy', $data) && $data['enabledBy'] === null) {
            $object->setEnabledBy(null);
        }
        if (\array_key_exists('items', $data) && $data['items'] !== null) {
            $values = [];
            foreach ($data['items'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\ItemResponse::class, 'json', $context);
            }
            $object->setItems($values);
        }
        elseif (\array_key_exists('items', $data) && $data['items'] === null) {
            $object->setItems(null);
        }
        if (\array_key_exists('linkedServiceAccountId', $data) && $data['linkedServiceAccountId'] !== null) {
            $object->setLinkedServiceAccountId($data['linkedServiceAccountId']);
        }
        elseif (\array_key_exists('linkedServiceAccountId', $data) && $data['linkedServiceAccountId'] === null) {
            $object->setLinkedServiceAccountId(null);
        }
        if (\array_key_exists('store', $data) && $data['store'] !== null) {
            $object->setStore($data['store']);
        }
        elseif (\array_key_exists('store', $data) && $data['store'] === null) {
            $object->setStore(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['available'] = $data->getAvailable();
        if ($data->isInitialized('basePath') && null !== $data->getBasePath()) {
            $dataArray['basePath'] = $data->getBasePath();
        }
        $dataArray['enabled'] = $data->getEnabled();
        if ($data->isInitialized('enabledAt') && null !== $data->getEnabledAt()) {
            $dataArray['enabledAt'] = $data->getEnabledAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('enabledBy') && null !== $data->getEnabledBy()) {
            $dataArray['enabledBy'] = $data->getEnabledBy();
        }
        $values = [];
        foreach ($data->getItems() as $value) {
            $values[] = $value === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($value, 'json', $context));
        }
        $dataArray['items'] = $values;
        if ($data->isInitialized('linkedServiceAccountId') && null !== $data->getLinkedServiceAccountId()) {
            $dataArray['linkedServiceAccountId'] = $data->getLinkedServiceAccountId();
        }
        if ($data->isInitialized('store') && null !== $data->getStore()) {
            $dataArray['store'] = $data->getStore();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PublicationResponse::class => false];
    }
}