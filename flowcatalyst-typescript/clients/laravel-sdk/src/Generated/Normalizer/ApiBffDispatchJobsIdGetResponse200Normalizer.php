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
class ApiBffDispatchJobsIdGetResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse200();
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
        if (\array_key_exists('externalId', $data) && $data['externalId'] !== null) {
            $object->setExternalId($data['externalId']);
            unset($data['externalId']);
        }
        elseif (\array_key_exists('externalId', $data) && $data['externalId'] === null) {
            $object->setExternalId(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
            unset($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
            unset($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
            unset($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('subject', $data) && $data['subject'] !== null) {
            $object->setSubject($data['subject']);
            unset($data['subject']);
        }
        elseif (\array_key_exists('subject', $data) && $data['subject'] === null) {
            $object->setSubject(null);
        }
        if (\array_key_exists('eventId', $data) && $data['eventId'] !== null) {
            $object->setEventId($data['eventId']);
            unset($data['eventId']);
        }
        elseif (\array_key_exists('eventId', $data) && $data['eventId'] === null) {
            $object->setEventId(null);
        }
        if (\array_key_exists('correlationId', $data) && $data['correlationId'] !== null) {
            $object->setCorrelationId($data['correlationId']);
            unset($data['correlationId']);
        }
        elseif (\array_key_exists('correlationId', $data) && $data['correlationId'] === null) {
            $object->setCorrelationId(null);
        }
        if (\array_key_exists('targetUrl', $data) && $data['targetUrl'] !== null) {
            $object->setTargetUrl($data['targetUrl']);
            unset($data['targetUrl']);
        }
        elseif (\array_key_exists('targetUrl', $data) && $data['targetUrl'] === null) {
            $object->setTargetUrl(null);
        }
        if (\array_key_exists('protocol', $data) && $data['protocol'] !== null) {
            $object->setProtocol($data['protocol']);
            unset($data['protocol']);
        }
        elseif (\array_key_exists('protocol', $data) && $data['protocol'] === null) {
            $object->setProtocol(null);
        }
        if (\array_key_exists('application', $data) && $data['application'] !== null) {
            $object->setApplication($data['application']);
            unset($data['application']);
        }
        elseif (\array_key_exists('application', $data) && $data['application'] === null) {
            $object->setApplication(null);
        }
        if (\array_key_exists('subdomain', $data) && $data['subdomain'] !== null) {
            $object->setSubdomain($data['subdomain']);
            unset($data['subdomain']);
        }
        elseif (\array_key_exists('subdomain', $data) && $data['subdomain'] === null) {
            $object->setSubdomain(null);
        }
        if (\array_key_exists('aggregate', $data) && $data['aggregate'] !== null) {
            $object->setAggregate($data['aggregate']);
            unset($data['aggregate']);
        }
        elseif (\array_key_exists('aggregate', $data) && $data['aggregate'] === null) {
            $object->setAggregate(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
            unset($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('subscriptionId', $data) && $data['subscriptionId'] !== null) {
            $object->setSubscriptionId($data['subscriptionId']);
            unset($data['subscriptionId']);
        }
        elseif (\array_key_exists('subscriptionId', $data) && $data['subscriptionId'] === null) {
            $object->setSubscriptionId(null);
        }
        if (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] !== null) {
            $object->setServiceAccountId($data['serviceAccountId']);
            unset($data['serviceAccountId']);
        }
        elseif (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] === null) {
            $object->setServiceAccountId(null);
        }
        if (\array_key_exists('dispatchPoolId', $data) && $data['dispatchPoolId'] !== null) {
            $object->setDispatchPoolId($data['dispatchPoolId']);
            unset($data['dispatchPoolId']);
        }
        elseif (\array_key_exists('dispatchPoolId', $data) && $data['dispatchPoolId'] === null) {
            $object->setDispatchPoolId(null);
        }
        if (\array_key_exists('messageGroup', $data) && $data['messageGroup'] !== null) {
            $object->setMessageGroup($data['messageGroup']);
            unset($data['messageGroup']);
        }
        elseif (\array_key_exists('messageGroup', $data) && $data['messageGroup'] === null) {
            $object->setMessageGroup(null);
        }
        if (\array_key_exists('mode', $data) && $data['mode'] !== null) {
            $object->setMode($data['mode']);
            unset($data['mode']);
        }
        elseif (\array_key_exists('mode', $data) && $data['mode'] === null) {
            $object->setMode(null);
        }
        if (\array_key_exists('sequence', $data) && $data['sequence'] !== null) {
            $object->setSequence($data['sequence']);
            unset($data['sequence']);
        }
        elseif (\array_key_exists('sequence', $data) && $data['sequence'] === null) {
            $object->setSequence(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
            unset($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('attemptCount', $data) && $data['attemptCount'] !== null) {
            $object->setAttemptCount($data['attemptCount']);
            unset($data['attemptCount']);
        }
        elseif (\array_key_exists('attemptCount', $data) && $data['attemptCount'] === null) {
            $object->setAttemptCount(null);
        }
        if (\array_key_exists('maxRetries', $data) && $data['maxRetries'] !== null) {
            $object->setMaxRetries($data['maxRetries']);
            unset($data['maxRetries']);
        }
        elseif (\array_key_exists('maxRetries', $data) && $data['maxRetries'] === null) {
            $object->setMaxRetries(null);
        }
        if (\array_key_exists('lastError', $data) && $data['lastError'] !== null) {
            $object->setLastError($data['lastError']);
            unset($data['lastError']);
        }
        elseif (\array_key_exists('lastError', $data) && $data['lastError'] === null) {
            $object->setLastError(null);
        }
        if (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] !== null) {
            $object->setTimeoutSeconds($data['timeoutSeconds']);
            unset($data['timeoutSeconds']);
        }
        elseif (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] === null) {
            $object->setTimeoutSeconds(null);
        }
        if (\array_key_exists('retryStrategy', $data) && $data['retryStrategy'] !== null) {
            $object->setRetryStrategy($data['retryStrategy']);
            unset($data['retryStrategy']);
        }
        elseif (\array_key_exists('retryStrategy', $data) && $data['retryStrategy'] === null) {
            $object->setRetryStrategy(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
            unset($data['updatedAt']);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        if (\array_key_exists('scheduledFor', $data) && $data['scheduledFor'] !== null) {
            $object->setScheduledFor($data['scheduledFor']);
            unset($data['scheduledFor']);
        }
        elseif (\array_key_exists('scheduledFor', $data) && $data['scheduledFor'] === null) {
            $object->setScheduledFor(null);
        }
        if (\array_key_exists('expiresAt', $data) && $data['expiresAt'] !== null) {
            $object->setExpiresAt($data['expiresAt']);
            unset($data['expiresAt']);
        }
        elseif (\array_key_exists('expiresAt', $data) && $data['expiresAt'] === null) {
            $object->setExpiresAt(null);
        }
        if (\array_key_exists('completedAt', $data) && $data['completedAt'] !== null) {
            $object->setCompletedAt($data['completedAt']);
            unset($data['completedAt']);
        }
        elseif (\array_key_exists('completedAt', $data) && $data['completedAt'] === null) {
            $object->setCompletedAt(null);
        }
        if (\array_key_exists('lastAttemptAt', $data) && $data['lastAttemptAt'] !== null) {
            $object->setLastAttemptAt($data['lastAttemptAt']);
            unset($data['lastAttemptAt']);
        }
        elseif (\array_key_exists('lastAttemptAt', $data) && $data['lastAttemptAt'] === null) {
            $object->setLastAttemptAt(null);
        }
        if (\array_key_exists('durationMillis', $data) && $data['durationMillis'] !== null) {
            $object->setDurationMillis($data['durationMillis']);
            unset($data['durationMillis']);
        }
        elseif (\array_key_exists('durationMillis', $data) && $data['durationMillis'] === null) {
            $object->setDurationMillis(null);
        }
        if (\array_key_exists('idempotencyKey', $data) && $data['idempotencyKey'] !== null) {
            $object->setIdempotencyKey($data['idempotencyKey']);
            unset($data['idempotencyKey']);
        }
        elseif (\array_key_exists('idempotencyKey', $data) && $data['idempotencyKey'] === null) {
            $object->setIdempotencyKey(null);
        }
        if (\array_key_exists('isCompleted', $data) && $data['isCompleted'] !== null) {
            $object->setIsCompleted($data['isCompleted']);
            unset($data['isCompleted']);
        }
        elseif (\array_key_exists('isCompleted', $data) && $data['isCompleted'] === null) {
            $object->setIsCompleted(null);
        }
        if (\array_key_exists('isTerminal', $data) && $data['isTerminal'] !== null) {
            $object->setIsTerminal($data['isTerminal']);
            unset($data['isTerminal']);
        }
        elseif (\array_key_exists('isTerminal', $data) && $data['isTerminal'] === null) {
            $object->setIsTerminal(null);
        }
        if (\array_key_exists('projectedAt', $data) && $data['projectedAt'] !== null) {
            $object->setProjectedAt($data['projectedAt']);
            unset($data['projectedAt']);
        }
        elseif (\array_key_exists('projectedAt', $data) && $data['projectedAt'] === null) {
            $object->setProjectedAt(null);
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
        $dataArray['externalId'] = $data->getExternalId();
        $dataArray['source'] = $data->getSource();
        $dataArray['kind'] = $data->getKind();
        $dataArray['code'] = $data->getCode();
        $dataArray['subject'] = $data->getSubject();
        $dataArray['eventId'] = $data->getEventId();
        $dataArray['correlationId'] = $data->getCorrelationId();
        $dataArray['targetUrl'] = $data->getTargetUrl();
        $dataArray['protocol'] = $data->getProtocol();
        $dataArray['application'] = $data->getApplication();
        $dataArray['subdomain'] = $data->getSubdomain();
        $dataArray['aggregate'] = $data->getAggregate();
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['subscriptionId'] = $data->getSubscriptionId();
        $dataArray['serviceAccountId'] = $data->getServiceAccountId();
        $dataArray['dispatchPoolId'] = $data->getDispatchPoolId();
        $dataArray['messageGroup'] = $data->getMessageGroup();
        $dataArray['mode'] = $data->getMode();
        $dataArray['sequence'] = $data->getSequence();
        $dataArray['status'] = $data->getStatus();
        $dataArray['attemptCount'] = $data->getAttemptCount();
        $dataArray['maxRetries'] = $data->getMaxRetries();
        $dataArray['lastError'] = $data->getLastError();
        $dataArray['timeoutSeconds'] = $data->getTimeoutSeconds();
        $dataArray['retryStrategy'] = $data->getRetryStrategy();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['scheduledFor'] = $data->getScheduledFor();
        $dataArray['expiresAt'] = $data->getExpiresAt();
        $dataArray['completedAt'] = $data->getCompletedAt();
        $dataArray['lastAttemptAt'] = $data->getLastAttemptAt();
        $dataArray['durationMillis'] = $data->getDurationMillis();
        $dataArray['idempotencyKey'] = $data->getIdempotencyKey();
        $dataArray['isCompleted'] = $data->getIsCompleted();
        $dataArray['isTerminal'] = $data->getIsTerminal();
        $dataArray['projectedAt'] = $data->getProjectedAt();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse200::class => false];
    }
}