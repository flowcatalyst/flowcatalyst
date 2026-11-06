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
class ScheduledJobInstanceResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ScheduledJobInstanceResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ScheduledJobInstanceResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\ScheduledJobInstanceResponse();
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
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('completedAt', $data) && $data['completedAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['completedAt']);
            if (false === $date) {
                if (is_string($data['completedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['completedAt'])) {
                    try {
                        $date = new \DateTime($data['completedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['completedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['completedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setCompletedAt($date);
        }
        elseif (\array_key_exists('completedAt', $data) && $data['completedAt'] === null) {
            $object->setCompletedAt(null);
        }
        if (\array_key_exists('completionResult', $data) && $data['completionResult'] !== null) {
            $object->setCompletionResult($data['completionResult']);
        }
        elseif (\array_key_exists('completionResult', $data) && $data['completionResult'] === null) {
            $object->setCompletionResult(null);
        }
        if (\array_key_exists('completionStatus', $data) && $data['completionStatus'] !== null) {
            $object->setCompletionStatus($data['completionStatus']);
        }
        elseif (\array_key_exists('completionStatus', $data) && $data['completionStatus'] === null) {
            $object->setCompletionStatus(null);
        }
        if (\array_key_exists('correlationId', $data) && $data['correlationId'] !== null) {
            $object->setCorrelationId($data['correlationId']);
        }
        elseif (\array_key_exists('correlationId', $data) && $data['correlationId'] === null) {
            $object->setCorrelationId(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']);
            if (false === $date_1) {
                if (is_string($data['createdAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['createdAt'])) {
                    try {
                        $date_1 = new \DateTime($data['createdAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['createdAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['createdAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setCreatedAt($date_1);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('deliveredAt', $data) && $data['deliveredAt'] !== null) {
            $date_2 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['deliveredAt']);
            if (false === $date_2) {
                if (is_string($data['deliveredAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['deliveredAt'])) {
                    try {
                        $date_2 = new \DateTime($data['deliveredAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['deliveredAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['deliveredAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setDeliveredAt($date_2);
        }
        elseif (\array_key_exists('deliveredAt', $data) && $data['deliveredAt'] === null) {
            $object->setDeliveredAt(null);
        }
        if (\array_key_exists('deliveryAttempts', $data) && $data['deliveryAttempts'] !== null) {
            $object->setDeliveryAttempts($data['deliveryAttempts']);
        }
        elseif (\array_key_exists('deliveryAttempts', $data) && $data['deliveryAttempts'] === null) {
            $object->setDeliveryAttempts(null);
        }
        if (\array_key_exists('deliveryError', $data) && $data['deliveryError'] !== null) {
            $object->setDeliveryError($data['deliveryError']);
        }
        elseif (\array_key_exists('deliveryError', $data) && $data['deliveryError'] === null) {
            $object->setDeliveryError(null);
        }
        if (\array_key_exists('firedAt', $data) && $data['firedAt'] !== null) {
            $date_3 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['firedAt']);
            if (false === $date_3) {
                if (is_string($data['firedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['firedAt'])) {
                    try {
                        $date_3 = new \DateTime($data['firedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['firedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['firedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setFiredAt($date_3);
        }
        elseif (\array_key_exists('firedAt', $data) && $data['firedAt'] === null) {
            $object->setFiredAt(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('jobCode', $data) && $data['jobCode'] !== null) {
            $object->setJobCode($data['jobCode']);
        }
        elseif (\array_key_exists('jobCode', $data) && $data['jobCode'] === null) {
            $object->setJobCode(null);
        }
        if (\array_key_exists('scheduledFor', $data) && $data['scheduledFor'] !== null) {
            $date_4 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['scheduledFor']);
            if (false === $date_4) {
                if (is_string($data['scheduledFor']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['scheduledFor'])) {
                    try {
                        $date_4 = new \DateTime($data['scheduledFor']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['scheduledFor'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['scheduledFor'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setScheduledFor($date_4);
        }
        elseif (\array_key_exists('scheduledFor', $data) && $data['scheduledFor'] === null) {
            $object->setScheduledFor(null);
        }
        if (\array_key_exists('scheduledJobId', $data) && $data['scheduledJobId'] !== null) {
            $object->setScheduledJobId($data['scheduledJobId']);
        }
        elseif (\array_key_exists('scheduledJobId', $data) && $data['scheduledJobId'] === null) {
            $object->setScheduledJobId(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('triggerKind', $data) && $data['triggerKind'] !== null) {
            $object->setTriggerKind($data['triggerKind']);
        }
        elseif (\array_key_exists('triggerKind', $data) && $data['triggerKind'] === null) {
            $object->setTriggerKind(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['clientId'] = $data->getClientId();
        }
        if ($data->isInitialized('completedAt') && null !== $data->getCompletedAt()) {
            $dataArray['completedAt'] = $data->getCompletedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('completionResult') && null !== $data->getCompletionResult()) {
            $dataArray['completionResult'] = $data->getCompletionResult();
        }
        if ($data->isInitialized('completionStatus') && null !== $data->getCompletionStatus()) {
            $dataArray['completionStatus'] = $data->getCompletionStatus();
        }
        if ($data->isInitialized('correlationId') && null !== $data->getCorrelationId()) {
            $dataArray['correlationId'] = $data->getCorrelationId();
        }
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('deliveredAt') && null !== $data->getDeliveredAt()) {
            $dataArray['deliveredAt'] = $data->getDeliveredAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['deliveryAttempts'] = $data->getDeliveryAttempts();
        if ($data->isInitialized('deliveryError') && null !== $data->getDeliveryError()) {
            $dataArray['deliveryError'] = $data->getDeliveryError();
        }
        $dataArray['firedAt'] = $data->getFiredAt()->format('Y-m-d\TH:i:sP');
        $dataArray['id'] = $data->getId();
        $dataArray['jobCode'] = $data->getJobCode();
        if ($data->isInitialized('scheduledFor') && null !== $data->getScheduledFor()) {
            $dataArray['scheduledFor'] = $data->getScheduledFor()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['scheduledJobId'] = $data->getScheduledJobId();
        $dataArray['status'] = $data->getStatus();
        $dataArray['triggerKind'] = $data->getTriggerKind();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ScheduledJobInstanceResponse::class => false];
    }
}