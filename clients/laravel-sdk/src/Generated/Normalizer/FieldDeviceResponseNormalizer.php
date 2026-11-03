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
class FieldDeviceResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FieldDeviceResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FieldDeviceResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FieldDeviceResponse();
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
        if (\array_key_exists('activeSessions', $data) && $data['activeSessions'] !== null) {
            $object->setActiveSessions($data['activeSessions']);
        }
        elseif (\array_key_exists('activeSessions', $data) && $data['activeSessions'] === null) {
            $object->setActiveSessions(null);
        }
        if (\array_key_exists('apps', $data) && $data['apps'] !== null) {
            $values = [];
            foreach ($data['apps'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\FieldAppRef::class, 'json', $context);
            }
            $object->setApps($values);
        }
        elseif (\array_key_exists('apps', $data) && $data['apps'] === null) {
            $object->setApps(null);
        }
        if (\array_key_exists('assignedWorkerId', $data) && $data['assignedWorkerId'] !== null) {
            $object->setAssignedWorkerId($data['assignedWorkerId']);
        }
        elseif (\array_key_exists('assignedWorkerId', $data) && $data['assignedWorkerId'] === null) {
            $object->setAssignedWorkerId(null);
        }
        if (\array_key_exists('attestationLevel', $data) && $data['attestationLevel'] !== null) {
            $object->setAttestationLevel($data['attestationLevel']);
        }
        elseif (\array_key_exists('attestationLevel', $data) && $data['attestationLevel'] === null) {
            $object->setAttestationLevel(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('enrolledAt', $data) && $data['enrolledAt'] !== null) {
            $object->setEnrolledAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['enrolledAt']));
        }
        elseif (\array_key_exists('enrolledAt', $data) && $data['enrolledAt'] === null) {
            $object->setEnrolledAt(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('jkt', $data) && $data['jkt'] !== null) {
            $object->setJkt($data['jkt']);
        }
        elseif (\array_key_exists('jkt', $data) && $data['jkt'] === null) {
            $object->setJkt(null);
        }
        if (\array_key_exists('label', $data) && $data['label'] !== null) {
            $object->setLabel($data['label']);
        }
        elseif (\array_key_exists('label', $data) && $data['label'] === null) {
            $object->setLabel(null);
        }
        if (\array_key_exists('lastSeenAt', $data) && $data['lastSeenAt'] !== null) {
            $object->setLastSeenAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['lastSeenAt']));
        }
        elseif (\array_key_exists('lastSeenAt', $data) && $data['lastSeenAt'] === null) {
            $object->setLastSeenAt(null);
        }
        if (\array_key_exists('mode', $data) && $data['mode'] !== null) {
            $object->setMode($data['mode']);
        }
        elseif (\array_key_exists('mode', $data) && $data['mode'] === null) {
            $object->setMode(null);
        }
        if (\array_key_exists('model', $data) && $data['model'] !== null) {
            $object->setModel($data['model']);
        }
        elseif (\array_key_exists('model', $data) && $data['model'] === null) {
            $object->setModel(null);
        }
        if (\array_key_exists('platform', $data) && $data['platform'] !== null) {
            $object->setPlatform($data['platform']);
        }
        elseif (\array_key_exists('platform', $data) && $data['platform'] === null) {
            $object->setPlatform(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['activeSessions'] = $data->getActiveSessions();
        $values = [];
        foreach ($data->getApps() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['apps'] = $values;
        if ($data->isInitialized('assignedWorkerId') && null !== $data->getAssignedWorkerId()) {
            $dataArray['assignedWorkerId'] = $data->getAssignedWorkerId();
        }
        $dataArray['attestationLevel'] = $data->getAttestationLevel();
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['enrolledAt'] = $data->getEnrolledAt()->format('Y-m-d\TH:i:sP');
        $dataArray['id'] = $data->getId();
        $dataArray['jkt'] = $data->getJkt();
        if ($data->isInitialized('label') && null !== $data->getLabel()) {
            $dataArray['label'] = $data->getLabel();
        }
        if ($data->isInitialized('lastSeenAt') && null !== $data->getLastSeenAt()) {
            $dataArray['lastSeenAt'] = $data->getLastSeenAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['mode'] = $data->getMode();
        if ($data->isInitialized('model') && null !== $data->getModel()) {
            $dataArray['model'] = $data->getModel();
        }
        if ($data->isInitialized('platform') && null !== $data->getPlatform()) {
            $dataArray['platform'] = $data->getPlatform();
        }
        $dataArray['status'] = $data->getStatus();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FieldDeviceResponse::class => false];
    }
}