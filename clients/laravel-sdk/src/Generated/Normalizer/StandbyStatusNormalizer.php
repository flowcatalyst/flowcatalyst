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
class StandbyStatusNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\StandbyStatus::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\StandbyStatus::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\StandbyStatus();
        if (\array_key_exists('isLeader', $data) && \is_int($data['isLeader'])) {
            $data['isLeader'] = (bool) $data['isLeader'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('clusterMembers', $data) && $data['clusterMembers'] !== null) {
            $values = [];
            foreach ($data['clusterMembers'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\ClusterMember::class, 'json', $context);
            }
            $object->setClusterMembers($values);
            unset($data['clusterMembers']);
        }
        elseif (\array_key_exists('clusterMembers', $data) && $data['clusterMembers'] === null) {
            $object->setClusterMembers(null);
        }
        if (\array_key_exists('instanceId', $data) && $data['instanceId'] !== null) {
            $object->setInstanceId($data['instanceId']);
            unset($data['instanceId']);
        }
        elseif (\array_key_exists('instanceId', $data) && $data['instanceId'] === null) {
            $object->setInstanceId(null);
        }
        if (\array_key_exists('isLeader', $data) && $data['isLeader'] !== null) {
            $object->setIsLeader($data['isLeader']);
            unset($data['isLeader']);
        }
        elseif (\array_key_exists('isLeader', $data) && $data['isLeader'] === null) {
            $object->setIsLeader(null);
        }
        if (\array_key_exists('lastHeartbeat', $data) && $data['lastHeartbeat'] !== null) {
            $object->setLastHeartbeat($data['lastHeartbeat']);
            unset($data['lastHeartbeat']);
        }
        elseif (\array_key_exists('lastHeartbeat', $data) && $data['lastHeartbeat'] === null) {
            $object->setLastHeartbeat(null);
        }
        if (\array_key_exists('leaderId', $data) && $data['leaderId'] !== null) {
            $object->setLeaderId($data['leaderId']);
            unset($data['leaderId']);
        }
        elseif (\array_key_exists('leaderId', $data) && $data['leaderId'] === null) {
            $object->setLeaderId(null);
        }
        if (\array_key_exists('role', $data) && $data['role'] !== null) {
            $object->setRole($data['role']);
            unset($data['role']);
        }
        elseif (\array_key_exists('role', $data) && $data['role'] === null) {
            $object->setRole(null);
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
        $values = [];
        foreach ($data->getClusterMembers() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['clusterMembers'] = $values;
        $dataArray['instanceId'] = $data->getInstanceId();
        $dataArray['isLeader'] = $data->getIsLeader();
        if ($data->isInitialized('lastHeartbeat')) {
            $dataArray['lastHeartbeat'] = $data->getLastHeartbeat();
        }
        if ($data->isInitialized('leaderId')) {
            $dataArray['leaderId'] = $data->getLeaderId();
        }
        $dataArray['role'] = $data->getRole();
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\StandbyStatus::class => false];
    }
}