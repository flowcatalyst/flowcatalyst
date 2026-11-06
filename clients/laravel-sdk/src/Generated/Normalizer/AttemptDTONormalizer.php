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
class AttemptDTONormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AttemptDTO::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AttemptDTO::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\AttemptDTO();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('success', $data) && \is_int($data['success'])) {
            $data['success'] = (bool) $data['success'];
        }
        if (\array_key_exists('attemptNumber', $data) && $data['attemptNumber'] !== null) {
            $object->setAttemptNumber($data['attemptNumber']);
        }
        elseif (\array_key_exists('attemptNumber', $data) && $data['attemptNumber'] === null) {
            $object->setAttemptNumber(null);
        }
        if (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['attemptedAt']);
            if (false === $date) {
                if (is_string($data['attemptedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['attemptedAt'])) {
                    try {
                        $date = new \DateTime($data['attemptedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['attemptedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['attemptedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setAttemptedAt($date);
        }
        elseif (\array_key_exists('attemptedAt', $data) && $data['attemptedAt'] === null) {
            $object->setAttemptedAt(null);
        }
        if (\array_key_exists('completedAt', $data) && $data['completedAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['completedAt']);
            if (false === $date_1) {
                if (is_string($data['completedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['completedAt'])) {
                    try {
                        $date_1 = new \DateTime($data['completedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['completedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['completedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setCompletedAt($date_1);
        }
        elseif (\array_key_exists('completedAt', $data) && $data['completedAt'] === null) {
            $object->setCompletedAt(null);
        }
        if (\array_key_exists('durationMillis', $data) && $data['durationMillis'] !== null) {
            $object->setDurationMillis($data['durationMillis']);
        }
        elseif (\array_key_exists('durationMillis', $data) && $data['durationMillis'] === null) {
            $object->setDurationMillis(null);
        }
        if (\array_key_exists('errorMessage', $data) && $data['errorMessage'] !== null) {
            $object->setErrorMessage($data['errorMessage']);
        }
        elseif (\array_key_exists('errorMessage', $data) && $data['errorMessage'] === null) {
            $object->setErrorMessage(null);
        }
        if (\array_key_exists('errorType', $data) && $data['errorType'] !== null) {
            $object->setErrorType($data['errorType']);
        }
        elseif (\array_key_exists('errorType', $data) && $data['errorType'] === null) {
            $object->setErrorType(null);
        }
        if (\array_key_exists('request', $data) && $data['request'] !== null) {
            $object->setRequest($this->denormalizer->denormalize($data['request'], \FlowCatalyst\Generated\Model\RequestSummary::class, 'json', $context));
        }
        elseif (\array_key_exists('request', $data) && $data['request'] === null) {
            $object->setRequest(null);
        }
        if (\array_key_exists('responseBody', $data) && $data['responseBody'] !== null) {
            $object->setResponseBody($data['responseBody']);
        }
        elseif (\array_key_exists('responseBody', $data) && $data['responseBody'] === null) {
            $object->setResponseBody(null);
        }
        if (\array_key_exists('responseCode', $data) && $data['responseCode'] !== null) {
            $object->setResponseCode($data['responseCode']);
        }
        elseif (\array_key_exists('responseCode', $data) && $data['responseCode'] === null) {
            $object->setResponseCode(null);
        }
        if (\array_key_exists('success', $data) && $data['success'] !== null) {
            $object->setSuccess($data['success']);
        }
        elseif (\array_key_exists('success', $data) && $data['success'] === null) {
            $object->setSuccess(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['attemptNumber'] = $data->getAttemptNumber();
        $dataArray['attemptedAt'] = $data->getAttemptedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('completedAt') && null !== $data->getCompletedAt()) {
            $dataArray['completedAt'] = $data->getCompletedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('durationMillis') && null !== $data->getDurationMillis()) {
            $dataArray['durationMillis'] = $data->getDurationMillis();
        }
        if ($data->isInitialized('errorMessage') && null !== $data->getErrorMessage()) {
            $dataArray['errorMessage'] = $data->getErrorMessage();
        }
        if ($data->isInitialized('errorType') && null !== $data->getErrorType()) {
            $dataArray['errorType'] = $data->getErrorType();
        }
        if ($data->isInitialized('request') && null !== $data->getRequest()) {
            $dataArray['request'] = $data->getRequest() === null ? null : new \FlowCatalyst\Generated\Runtime\JsonObject($this->normalizer->normalize($data->getRequest(), 'json', $context));
        }
        if ($data->isInitialized('responseBody') && null !== $data->getResponseBody()) {
            $dataArray['responseBody'] = $data->getResponseBody();
        }
        if ($data->isInitialized('responseCode') && null !== $data->getResponseCode()) {
            $dataArray['responseCode'] = $data->getResponseCode();
        }
        $dataArray['success'] = $data->getSuccess();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AttemptDTO::class => false];
    }
}