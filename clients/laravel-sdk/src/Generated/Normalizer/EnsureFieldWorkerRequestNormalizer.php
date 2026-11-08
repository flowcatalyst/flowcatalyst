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
class EnsureFieldWorkerRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('canImpersonate', $data) && \is_int($data['canImpersonate'])) {
            $data['canImpersonate'] = (bool) $data['canImpersonate'];
        }
        if (\array_key_exists('isTest', $data) && \is_int($data['isTest'])) {
            $data['isTest'] = (bool) $data['isTest'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
            unset($data['$schema']);
        }
        if (\array_key_exists('appCodes', $data) && $data['appCodes'] !== null) {
            $values = [];
            foreach ($data['appCodes'] as $value) {
                $values[] = $value;
            }
            $object->setAppCodes($values);
            unset($data['appCodes']);
        }
        elseif (\array_key_exists('appCodes', $data) && $data['appCodes'] === null) {
            $object->setAppCodes(null);
            unset($data['appCodes']);
        }
        if (\array_key_exists('canImpersonate', $data) && $data['canImpersonate'] !== null) {
            $object->setCanImpersonate($data['canImpersonate']);
            unset($data['canImpersonate']);
        }
        elseif (\array_key_exists('canImpersonate', $data) && $data['canImpersonate'] === null) {
            $object->setCanImpersonate(null);
            unset($data['canImpersonate']);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
            unset($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
            unset($data['clientId']);
        }
        if (\array_key_exists('email', $data) && $data['email'] !== null) {
            $object->setEmail($data['email']);
            unset($data['email']);
        }
        elseif (\array_key_exists('email', $data) && $data['email'] === null) {
            $object->setEmail(null);
            unset($data['email']);
        }
        if (\array_key_exists('isTest', $data) && $data['isTest'] !== null) {
            $object->setIsTest($data['isTest']);
            unset($data['isTest']);
        }
        elseif (\array_key_exists('isTest', $data) && $data['isTest'] === null) {
            $object->setIsTest(null);
            unset($data['isTest']);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
            unset($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
            unset($data['name']);
        }
        if (\array_key_exists('phone', $data) && $data['phone'] !== null) {
            $object->setPhone($data['phone']);
            unset($data['phone']);
        }
        elseif (\array_key_exists('phone', $data) && $data['phone'] === null) {
            $object->setPhone(null);
            unset($data['phone']);
        }
        if (\array_key_exists('pin', $data) && $data['pin'] !== null) {
            $object->setPin($data['pin']);
            unset($data['pin']);
        }
        elseif (\array_key_exists('pin', $data) && $data['pin'] === null) {
            $object->setPin(null);
            unset($data['pin']);
        }
        if (\array_key_exists('workerCode', $data) && $data['workerCode'] !== null) {
            $object->setWorkerCode($data['workerCode']);
            unset($data['workerCode']);
        }
        elseif (\array_key_exists('workerCode', $data) && $data['workerCode'] === null) {
            $object->setWorkerCode(null);
            unset($data['workerCode']);
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
        if ($data->isInitialized('appCodes') && null !== $data->getAppCodes()) {
            $values = [];
            foreach ($data->getAppCodes() as $value) {
                $values[] = $value;
            }
            $dataArray['appCodes'] = $values;
        }
        if ($data->isInitialized('canImpersonate') && null !== $data->getCanImpersonate()) {
            $dataArray['canImpersonate'] = $data->getCanImpersonate();
        }
        $dataArray['clientId'] = $data->getClientId();
        if ($data->isInitialized('email') && null !== $data->getEmail()) {
            $dataArray['email'] = $data->getEmail();
        }
        if ($data->isInitialized('isTest') && null !== $data->getIsTest()) {
            $dataArray['isTest'] = $data->getIsTest();
        }
        $dataArray['name'] = $data->getName();
        if ($data->isInitialized('phone') && null !== $data->getPhone()) {
            $dataArray['phone'] = $data->getPhone();
        }
        if ($data->isInitialized('pin') && null !== $data->getPin()) {
            $dataArray['pin'] = $data->getPin();
        }
        $dataArray['workerCode'] = $data->getWorkerCode();
        foreach ($data->additionalPropertyEntries() as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest::class => false];
    }
}