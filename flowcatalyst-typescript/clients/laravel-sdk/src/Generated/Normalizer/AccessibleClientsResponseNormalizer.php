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
class AccessibleClientsResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AccessibleClientsResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AccessibleClientsResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\AccessibleClientsResponse();
        if (\array_key_exists('globalAccess', $data) && \is_int($data['globalAccess'])) {
            $data['globalAccess'] = (bool) $data['globalAccess'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('clients', $data) && $data['clients'] !== null) {
            $values = [];
            foreach ($data['clients'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\ClientInfo::class, 'json', $context);
            }
            $object->setClients($values);
            unset($data['clients']);
        }
        elseif (\array_key_exists('clients', $data) && $data['clients'] === null) {
            $object->setClients(null);
        }
        if (\array_key_exists('currentClientId', $data) && $data['currentClientId'] !== null) {
            $object->setCurrentClientId($data['currentClientId']);
            unset($data['currentClientId']);
        }
        elseif (\array_key_exists('currentClientId', $data) && $data['currentClientId'] === null) {
            $object->setCurrentClientId(null);
        }
        if (\array_key_exists('globalAccess', $data) && $data['globalAccess'] !== null) {
            $object->setGlobalAccess($data['globalAccess']);
            unset($data['globalAccess']);
        }
        elseif (\array_key_exists('globalAccess', $data) && $data['globalAccess'] === null) {
            $object->setGlobalAccess(null);
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
        if ($data->isInitialized('clients') && null !== $data->getClients()) {
            $values = [];
            foreach ($data->getClients() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['clients'] = $values;
        }
        if ($data->isInitialized('currentClientId') && null !== $data->getCurrentClientId()) {
            $dataArray['currentClientId'] = $data->getCurrentClientId();
        }
        if ($data->isInitialized('globalAccess') && null !== $data->getGlobalAccess()) {
            $dataArray['globalAccess'] = $data->getGlobalAccess();
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
        return [\FlowCatalyst\Generated\Model\AccessibleClientsResponse::class => false];
    }
}