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
class ClusterMemberNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ClusterMember::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ClusterMember::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ClusterMember();
        if (\array_key_exists('healthy', $data) && \is_int($data['healthy'])) {
            $data['healthy'] = (bool) $data['healthy'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('healthy', $data) && $data['healthy'] !== null) {
            $object->setHealthy($data['healthy']);
            unset($data['healthy']);
        }
        elseif (\array_key_exists('healthy', $data) && $data['healthy'] === null) {
            $object->setHealthy(null);
        }
        if (\array_key_exists('instanceId', $data) && $data['instanceId'] !== null) {
            $object->setInstanceId($data['instanceId']);
            unset($data['instanceId']);
        }
        elseif (\array_key_exists('instanceId', $data) && $data['instanceId'] === null) {
            $object->setInstanceId(null);
        }
        if (\array_key_exists('lastSeen', $data) && $data['lastSeen'] !== null) {
            $object->setLastSeen($data['lastSeen']);
            unset($data['lastSeen']);
        }
        elseif (\array_key_exists('lastSeen', $data) && $data['lastSeen'] === null) {
            $object->setLastSeen(null);
        }
        if (\array_key_exists('role', $data) && $data['role'] !== null) {
            $object->setRole($data['role']);
            unset($data['role']);
        }
        elseif (\array_key_exists('role', $data) && $data['role'] === null) {
            $object->setRole(null);
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['healthy'] = $data->getHealthy();
        $dataArray['instanceId'] = $data->getInstanceId();
        $dataArray['lastSeen'] = $data->getLastSeen();
        $dataArray['role'] = $data->getRole();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ClusterMember::class => false];
    }
}