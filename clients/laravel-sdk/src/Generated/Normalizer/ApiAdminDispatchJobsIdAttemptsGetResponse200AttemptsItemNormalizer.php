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
class ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItemNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem();
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
        if (\array_key_exists('durationMillis', $data) && $data['durationMillis'] !== null) {
            $object->setDurationMillis($data['durationMillis']);
            unset($data['durationMillis']);
        }
        elseif (\array_key_exists('durationMillis', $data) && $data['durationMillis'] === null) {
            $object->setDurationMillis(null);
        }
        if (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] !== null) {
            $object->setAttemptedAt($data['attemptedAt']);
            unset($data['attemptedAt']);
        }
        elseif (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] === null) {
            $object->setAttemptedAt(null);
        }
        if (\array_key_exists('completedAt', $data) && $data['completedAt'] !== null) {
            $object->setCompletedAt($data['completedAt']);
            unset($data['completedAt']);
        }
        elseif (\array_key_exists('completedAt', $data) && $data['completedAt'] === null) {
            $object->setCompletedAt(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt($data['createdAt']);
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
        $dataArray['id'] = $data->getId();
        $dataArray['attemptNumber'] = $data->getAttemptNumber();
        $dataArray['status'] = $data->getStatus();
        $dataArray['responseCode'] = $data->getResponseCode();
        $dataArray['responseBody'] = $data->getResponseBody();
        $dataArray['errorMessage'] = $data->getErrorMessage();
        $dataArray['errorType'] = $data->getErrorType();
        $dataArray['durationMillis'] = $data->getDurationMillis();
        $dataArray['attemptedAt'] = $data->getAttemptedAt();
        $dataArray['completedAt'] = $data->getCompletedAt();
        $dataArray['createdAt'] = $data->getCreatedAt();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem::class => false];
    }
}