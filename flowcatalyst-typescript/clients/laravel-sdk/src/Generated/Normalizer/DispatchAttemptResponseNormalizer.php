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
class DispatchAttemptResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\DispatchAttemptResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\DispatchAttemptResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\DispatchAttemptResponse();
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
        if (\array_key_exists('attemptNumber', $data) && $data['attemptNumber'] !== null) {
            $object->setAttemptNumber($data['attemptNumber']);
            unset($data['attemptNumber']);
        }
        elseif (\array_key_exists('attemptNumber', $data) && $data['attemptNumber'] === null) {
            $object->setAttemptNumber(null);
        }
        if (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] !== null) {
            $object->setAttemptedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['attemptedAt']));
            unset($data['attemptedAt']);
        }
        elseif (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] === null) {
            $object->setAttemptedAt(null);
        }
        if (\array_key_exists('completedAt', $data) && $data['completedAt'] !== null) {
            $object->setCompletedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['completedAt']));
            unset($data['completedAt']);
        }
        elseif (\array_key_exists('completedAt', $data) && $data['completedAt'] === null) {
            $object->setCompletedAt(null);
        }
        if (\array_key_exists('durationMillis', $data) && $data['durationMillis'] !== null) {
            $object->setDurationMillis($data['durationMillis']);
            unset($data['durationMillis']);
        }
        elseif (\array_key_exists('durationMillis', $data) && $data['durationMillis'] === null) {
            $object->setDurationMillis(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
            unset($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('responseCode', $data) && $data['responseCode'] !== null) {
            $object->setResponseCode($data['responseCode']);
            unset($data['responseCode']);
        }
        elseif (\array_key_exists('responseCode', $data) && $data['responseCode'] === null) {
            $object->setResponseCode(null);
        }
        if (\array_key_exists('responseBody', $data) && $data['responseBody'] !== null) {
            $object->setResponseBody($data['responseBody']);
            unset($data['responseBody']);
        }
        elseif (\array_key_exists('responseBody', $data) && $data['responseBody'] === null) {
            $object->setResponseBody(null);
        }
        if (\array_key_exists('errorMessage', $data) && $data['errorMessage'] !== null) {
            $object->setErrorMessage($data['errorMessage']);
            unset($data['errorMessage']);
        }
        elseif (\array_key_exists('errorMessage', $data) && $data['errorMessage'] === null) {
            $object->setErrorMessage(null);
        }
        if (\array_key_exists('errorType', $data) && $data['errorType'] !== null) {
            $object->setErrorType($data['errorType']);
            unset($data['errorType']);
        }
        elseif (\array_key_exists('errorType', $data) && $data['errorType'] === null) {
            $object->setErrorType(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
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
        if ($data->isInitialized('id') && null !== $data->getId()) {
            $dataArray['id'] = $data->getId();
        }
        if ($data->isInitialized('attemptNumber') && null !== $data->getAttemptNumber()) {
            $dataArray['attemptNumber'] = $data->getAttemptNumber();
        }
        if ($data->isInitialized('attemptedAt') && null !== $data->getAttemptedAt()) {
            $dataArray['attemptedAt'] = $data->getAttemptedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('completedAt') && null !== $data->getCompletedAt()) {
            $dataArray['completedAt'] = $data->getCompletedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('durationMillis') && null !== $data->getDurationMillis()) {
            $dataArray['durationMillis'] = $data->getDurationMillis();
        }
        if ($data->isInitialized('status') && null !== $data->getStatus()) {
            $dataArray['status'] = $data->getStatus();
        }
        if ($data->isInitialized('responseCode') && null !== $data->getResponseCode()) {
            $dataArray['responseCode'] = $data->getResponseCode();
        }
        if ($data->isInitialized('responseBody') && null !== $data->getResponseBody()) {
            $dataArray['responseBody'] = $data->getResponseBody();
        }
        if ($data->isInitialized('errorMessage') && null !== $data->getErrorMessage()) {
            $dataArray['errorMessage'] = $data->getErrorMessage();
        }
        if ($data->isInitialized('errorType') && null !== $data->getErrorType()) {
            $dataArray['errorType'] = $data->getErrorType();
        }
        if ($data->isInitialized('createdAt') && null !== $data->getCreatedAt()) {
            $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\DispatchAttemptResponse::class => false];
    }
}