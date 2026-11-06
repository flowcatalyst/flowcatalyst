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
class FieldTokenIntrospectResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FieldTokenIntrospectResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FieldTokenIntrospectResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FieldTokenIntrospectResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('active', $data) && \is_int($data['active'])) {
            $data['active'] = (bool) $data['active'];
        }
        if (\array_key_exists('test', $data) && \is_int($data['test'])) {
            $data['test'] = (bool) $data['test'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('act', $data) && $data['act'] !== null) {
            $object->setAct($this->denormalizer->denormalize($data['act'], \FlowCatalyst\Generated\Model\FieldTokenActor::class, 'json', $context));
        }
        elseif (\array_key_exists('act', $data) && $data['act'] === null) {
            $object->setAct(null);
        }
        if (\array_key_exists('active', $data) && $data['active'] !== null) {
            $object->setActive($data['active']);
        }
        elseif (\array_key_exists('active', $data) && $data['active'] === null) {
            $object->setActive(null);
        }
        if (\array_key_exists('amr', $data) && $data['amr'] !== null) {
            $values = [];
            foreach ($data['amr'] as $value) {
                $values[] = $value;
            }
            $object->setAmr($values);
        }
        elseif (\array_key_exists('amr', $data) && $data['amr'] === null) {
            $object->setAmr(null);
        }
        if (\array_key_exists('appCode', $data) && $data['appCode'] !== null) {
            $object->setAppCode($data['appCode']);
        }
        elseif (\array_key_exists('appCode', $data) && $data['appCode'] === null) {
            $object->setAppCode(null);
        }
        if (\array_key_exists('authTime', $data) && $data['authTime'] !== null) {
            $object->setAuthTime(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['authTime']));
        }
        elseif (\array_key_exists('authTime', $data) && $data['authTime'] === null) {
            $object->setAuthTime(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('deviceId', $data) && $data['deviceId'] !== null) {
            $object->setDeviceId($data['deviceId']);
        }
        elseif (\array_key_exists('deviceId', $data) && $data['deviceId'] === null) {
            $object->setDeviceId(null);
        }
        if (\array_key_exists('exp', $data) && $data['exp'] !== null) {
            $object->setExp(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['exp']));
        }
        elseif (\array_key_exists('exp', $data) && $data['exp'] === null) {
            $object->setExp(null);
        }
        if (\array_key_exists('sub', $data) && $data['sub'] !== null) {
            $object->setSub($data['sub']);
        }
        elseif (\array_key_exists('sub', $data) && $data['sub'] === null) {
            $object->setSub(null);
        }
        if (\array_key_exists('test', $data) && $data['test'] !== null) {
            $object->setTest($data['test']);
        }
        elseif (\array_key_exists('test', $data) && $data['test'] === null) {
            $object->setTest(null);
        }
        if (\array_key_exists('workerCode', $data) && $data['workerCode'] !== null) {
            $object->setWorkerCode($data['workerCode']);
        }
        elseif (\array_key_exists('workerCode', $data) && $data['workerCode'] === null) {
            $object->setWorkerCode(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('act') && null !== $data->getAct()) {
            $dataArray['act'] = $this->normalizer->normalize($data->getAct(), 'json', $context);
        }
        $dataArray['active'] = $data->getActive();
        if ($data->isInitialized('amr') && null !== $data->getAmr()) {
            $values = [];
            foreach ($data->getAmr() as $value) {
                $values[] = $value;
            }
            $dataArray['amr'] = $values;
        }
        if ($data->isInitialized('appCode') && null !== $data->getAppCode()) {
            $dataArray['appCode'] = $data->getAppCode();
        }
        if ($data->isInitialized('authTime') && null !== $data->getAuthTime()) {
            $dataArray['authTime'] = $data->getAuthTime()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['clientId'] = $data->getClientId();
        }
        if ($data->isInitialized('deviceId') && null !== $data->getDeviceId()) {
            $dataArray['deviceId'] = $data->getDeviceId();
        }
        if ($data->isInitialized('exp') && null !== $data->getExp()) {
            $dataArray['exp'] = $data->getExp()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('sub') && null !== $data->getSub()) {
            $dataArray['sub'] = $data->getSub();
        }
        if ($data->isInitialized('test') && null !== $data->getTest()) {
            $dataArray['test'] = $data->getTest();
        }
        if ($data->isInitialized('workerCode') && null !== $data->getWorkerCode()) {
            $dataArray['workerCode'] = $data->getWorkerCode();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FieldTokenIntrospectResponse::class => false];
    }
}