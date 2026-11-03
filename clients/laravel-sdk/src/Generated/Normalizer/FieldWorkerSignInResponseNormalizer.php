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
class FieldWorkerSignInResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FieldWorkerSignInResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FieldWorkerSignInResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FieldWorkerSignInResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('appCode', $data) && $data['appCode'] !== null) {
            $object->setAppCode($data['appCode']);
        }
        elseif (\array_key_exists('appCode', $data) && $data['appCode'] === null) {
            $object->setAppCode(null);
        }
        if (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] !== null) {
            $object->setAttemptedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['attemptedAt']));
        }
        elseif (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] === null) {
            $object->setAttemptedAt(null);
        }
        if (\array_key_exists('authMethod', $data) && $data['authMethod'] !== null) {
            $object->setAuthMethod($data['authMethod']);
        }
        elseif (\array_key_exists('authMethod', $data) && $data['authMethod'] === null) {
            $object->setAuthMethod(null);
        }
        if (\array_key_exists('deviceId', $data) && $data['deviceId'] !== null) {
            $object->setDeviceId($data['deviceId']);
        }
        elseif (\array_key_exists('deviceId', $data) && $data['deviceId'] === null) {
            $object->setDeviceId(null);
        }
        if (\array_key_exists('failureReason', $data) && $data['failureReason'] !== null) {
            $object->setFailureReason($data['failureReason']);
        }
        elseif (\array_key_exists('failureReason', $data) && $data['failureReason'] === null) {
            $object->setFailureReason(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('ipAddress', $data) && $data['ipAddress'] !== null) {
            $object->setIpAddress($data['ipAddress']);
        }
        elseif (\array_key_exists('ipAddress', $data) && $data['ipAddress'] === null) {
            $object->setIpAddress(null);
        }
        if (\array_key_exists('outcome', $data) && $data['outcome'] !== null) {
            $object->setOutcome($data['outcome']);
        }
        elseif (\array_key_exists('outcome', $data) && $data['outcome'] === null) {
            $object->setOutcome(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('appCode') && null !== $data->getAppCode()) {
            $dataArray['appCode'] = $data->getAppCode();
        }
        $dataArray['attemptedAt'] = $data->getAttemptedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['authMethod'] = $data->getAuthMethod();
        if ($data->isInitialized('deviceId') && null !== $data->getDeviceId()) {
            $dataArray['deviceId'] = $data->getDeviceId();
        }
        if ($data->isInitialized('failureReason') && null !== $data->getFailureReason()) {
            $dataArray['failureReason'] = $data->getFailureReason();
        }
        $dataArray['id'] = $data->getId();
        if ($data->isInitialized('ipAddress') && null !== $data->getIpAddress()) {
            $dataArray['ipAddress'] = $data->getIpAddress();
        }
        $dataArray['outcome'] = $data->getOutcome();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FieldWorkerSignInResponse::class => false];
    }
}