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
class ApiAdminLoginAttemptsGetResponse200ItemsItemNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminLoginAttemptsGetResponse200ItemsItem::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminLoginAttemptsGetResponse200ItemsItem::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminLoginAttemptsGetResponse200ItemsItem();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
            unset($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('attemptType', $data) && $data['attemptType'] !== null) {
            $object->setAttemptType($data['attemptType']);
            unset($data['attemptType']);
        }
        elseif (\array_key_exists('attemptType', $data) && $data['attemptType'] === null) {
            $object->setAttemptType(null);
        }
        if (\array_key_exists('outcome', $data) && $data['outcome'] !== null) {
            $object->setOutcome($data['outcome']);
            unset($data['outcome']);
        }
        elseif (\array_key_exists('outcome', $data) && $data['outcome'] === null) {
            $object->setOutcome(null);
        }
        if (\array_key_exists('failureReason', $data) && $data['failureReason'] !== null) {
            $object->setFailureReason($data['failureReason']);
            unset($data['failureReason']);
        }
        elseif (\array_key_exists('failureReason', $data) && $data['failureReason'] === null) {
            $object->setFailureReason(null);
        }
        if (\array_key_exists('identifier', $data) && $data['identifier'] !== null) {
            $object->setIdentifier($data['identifier']);
            unset($data['identifier']);
        }
        elseif (\array_key_exists('identifier', $data) && $data['identifier'] === null) {
            $object->setIdentifier(null);
        }
        if (\array_key_exists('principalId', $data) && $data['principalId'] !== null) {
            $object->setPrincipalId($data['principalId']);
            unset($data['principalId']);
        }
        elseif (\array_key_exists('principalId', $data) && $data['principalId'] === null) {
            $object->setPrincipalId(null);
        }
        if (\array_key_exists('ipAddress', $data) && $data['ipAddress'] !== null) {
            $object->setIpAddress($data['ipAddress']);
            unset($data['ipAddress']);
        }
        elseif (\array_key_exists('ipAddress', $data) && $data['ipAddress'] === null) {
            $object->setIpAddress(null);
        }
        if (\array_key_exists('userAgent', $data) && $data['userAgent'] !== null) {
            $object->setUserAgent($data['userAgent']);
            unset($data['userAgent']);
        }
        elseif (\array_key_exists('userAgent', $data) && $data['userAgent'] === null) {
            $object->setUserAgent(null);
        }
        if (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] !== null) {
            $object->setAttemptedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['attemptedAt']));
            unset($data['attemptedAt']);
        }
        elseif (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] === null) {
            $object->setAttemptedAt(null);
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
        $dataArray['id'] = $data->getId();
        $dataArray['attemptType'] = $data->getAttemptType();
        $dataArray['outcome'] = $data->getOutcome();
        $dataArray['failureReason'] = $data->getFailureReason();
        $dataArray['identifier'] = $data->getIdentifier();
        $dataArray['principalId'] = $data->getPrincipalId();
        $dataArray['ipAddress'] = $data->getIpAddress();
        $dataArray['userAgent'] = $data->getUserAgent();
        $dataArray['attemptedAt'] = $data->getAttemptedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminLoginAttemptsGetResponse200ItemsItem::class => false];
    }
}