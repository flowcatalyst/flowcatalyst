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
class CreateDispatchJobRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\CreateDispatchJobRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\CreateDispatchJobRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\CreateDispatchJobRequest();
        if (\array_key_exists('dataOnly', $data) && \is_int($data['dataOnly'])) {
            $data['dataOnly'] = (bool) $data['dataOnly'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
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
        if (\array_key_exists('metadata', $data) && $data['metadata'] !== null) {
            $values = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['metadata'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setMetadata($values);
            unset($data['metadata']);
        }
        elseif (\array_key_exists('metadata', $data) && $data['metadata'] === null) {
            $object->setMetadata(null);
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
        if (\array_key_exists('headers', $data) && $data['headers'] !== null) {
            $values_1 = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['headers'] as $key_1 => $value_1) {
                $values_1[$key_1] = $value_1;
            }
            $object->setHeaders($values_1);
            unset($data['headers']);
        }
        elseif (\array_key_exists('headers', $data) && $data['headers'] === null) {
            $object->setHeaders(null);
        }
        if (\array_key_exists('payload', $data) && $data['payload'] !== null) {
            $object->setPayload($data['payload']);
            unset($data['payload']);
        }
        elseif (\array_key_exists('payload', $data) && $data['payload'] === null) {
            $object->setPayload(null);
        }
        if (\array_key_exists('payloadContentType', $data) && $data['payloadContentType'] !== null) {
            $object->setPayloadContentType($data['payloadContentType']);
            unset($data['payloadContentType']);
        }
        elseif (\array_key_exists('payloadContentType', $data) && $data['payloadContentType'] === null) {
            $object->setPayloadContentType(null);
        }
        if (\array_key_exists('dataOnly', $data) && $data['dataOnly'] !== null) {
            $object->setDataOnly($data['dataOnly']);
            unset($data['dataOnly']);
        }
        elseif (\array_key_exists('dataOnly', $data) && $data['dataOnly'] === null) {
            $object->setDataOnly(null);
        }
        if (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] !== null) {
            $object->setServiceAccountId($data['serviceAccountId']);
            unset($data['serviceAccountId']);
        }
        elseif (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] === null) {
            $object->setServiceAccountId(null);
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
        if (\array_key_exists('mode', $data) && $data['mode'] !== null) {
            $object->setMode($data['mode']);
            unset($data['mode']);
        }
        elseif (\array_key_exists('mode', $data) && $data['mode'] === null) {
            $object->setMode(null);
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
        if (\array_key_exists('sequence', $data) && $data['sequence'] !== null) {
            $object->setSequence($data['sequence']);
            unset($data['sequence']);
        }
        elseif (\array_key_exists('sequence', $data) && $data['sequence'] === null) {
            $object->setSequence(null);
        }
        if (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] !== null) {
            $object->setTimeoutSeconds($data['timeoutSeconds']);
            unset($data['timeoutSeconds']);
        }
        elseif (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] === null) {
            $object->setTimeoutSeconds(null);
        }
        if (\array_key_exists('schemaId', $data) && $data['schemaId'] !== null) {
            $object->setSchemaId($data['schemaId']);
            unset($data['schemaId']);
        }
        elseif (\array_key_exists('schemaId', $data) && $data['schemaId'] === null) {
            $object->setSchemaId(null);
        }
        if (\array_key_exists('maxRetries', $data) && $data['maxRetries'] !== null) {
            $object->setMaxRetries($data['maxRetries']);
            unset($data['maxRetries']);
        }
        elseif (\array_key_exists('maxRetries', $data) && $data['maxRetries'] === null) {
            $object->setMaxRetries(null);
        }
        if (\array_key_exists('retryStrategy', $data) && $data['retryStrategy'] !== null) {
            $object->setRetryStrategy($data['retryStrategy']);
            unset($data['retryStrategy']);
        }
        elseif (\array_key_exists('retryStrategy', $data) && $data['retryStrategy'] === null) {
            $object->setRetryStrategy(null);
        }
        if (\array_key_exists('scheduledFor', $data) && $data['scheduledFor'] !== null) {
            $object->setScheduledFor(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['scheduledFor']));
            unset($data['scheduledFor']);
        }
        elseif (\array_key_exists('scheduledFor', $data) && $data['scheduledFor'] === null) {
            $object->setScheduledFor(null);
        }
        if (\array_key_exists('expiresAt', $data) && $data['expiresAt'] !== null) {
            $object->setExpiresAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['expiresAt']));
            unset($data['expiresAt']);
        }
        elseif (\array_key_exists('expiresAt', $data) && $data['expiresAt'] === null) {
            $object->setExpiresAt(null);
        }
        if (\array_key_exists('idempotencyKey', $data) && $data['idempotencyKey'] !== null) {
            $object->setIdempotencyKey($data['idempotencyKey']);
            unset($data['idempotencyKey']);
        }
        elseif (\array_key_exists('idempotencyKey', $data) && $data['idempotencyKey'] === null) {
            $object->setIdempotencyKey(null);
        }
        if (\array_key_exists('externalId', $data) && $data['externalId'] !== null) {
            $object->setExternalId($data['externalId']);
            unset($data['externalId']);
        }
        elseif (\array_key_exists('externalId', $data) && $data['externalId'] === null) {
            $object->setExternalId(null);
        }
        if (\array_key_exists('queueUrl', $data) && $data['queueUrl'] !== null) {
            $object->setQueueUrl($data['queueUrl']);
            unset($data['queueUrl']);
        }
        elseif (\array_key_exists('queueUrl', $data) && $data['queueUrl'] === null) {
            $object->setQueueUrl(null);
        }
        foreach ($data as $key_2 => $value_2) {
            if (preg_match('/.*/', (string) $key_2)) {
                $object[$key_2] = $value_2;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['source'] = $data->getSource();
        if ($data->isInitialized('kind') && null !== $data->getKind()) {
            $dataArray['kind'] = $data->getKind();
        }
        $dataArray['code'] = $data->getCode();
        if ($data->isInitialized('subject') && null !== $data->getSubject()) {
            $dataArray['subject'] = $data->getSubject();
        }
        if ($data->isInitialized('eventId') && null !== $data->getEventId()) {
            $dataArray['eventId'] = $data->getEventId();
        }
        if ($data->isInitialized('correlationId') && null !== $data->getCorrelationId()) {
            $dataArray['correlationId'] = $data->getCorrelationId();
        }
        if ($data->isInitialized('metadata') && null !== $data->getMetadata()) {
            $values = [];
            foreach ($data->getMetadata() as $key => $value) {
                $values[$key] = $value;
            }
            $dataArray['metadata'] = $values;
        }
        $dataArray['targetUrl'] = $data->getTargetUrl();
        if ($data->isInitialized('protocol') && null !== $data->getProtocol()) {
            $dataArray['protocol'] = $data->getProtocol();
        }
        if ($data->isInitialized('headers') && null !== $data->getHeaders()) {
            $values_1 = [];
            foreach ($data->getHeaders() as $key_1 => $value_1) {
                $values_1[$key_1] = $value_1;
            }
            $dataArray['headers'] = $values_1;
        }
        $dataArray['payload'] = $data->getPayload();
        if ($data->isInitialized('payloadContentType') && null !== $data->getPayloadContentType()) {
            $dataArray['payloadContentType'] = $data->getPayloadContentType();
        }
        if ($data->isInitialized('dataOnly') && null !== $data->getDataOnly()) {
            $dataArray['dataOnly'] = $data->getDataOnly();
        }
        $dataArray['serviceAccountId'] = $data->getServiceAccountId();
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['clientId'] = $data->getClientId();
        }
        if ($data->isInitialized('subscriptionId') && null !== $data->getSubscriptionId()) {
            $dataArray['subscriptionId'] = $data->getSubscriptionId();
        }
        if ($data->isInitialized('mode') && null !== $data->getMode()) {
            $dataArray['mode'] = $data->getMode();
        }
        if ($data->isInitialized('dispatchPoolId') && null !== $data->getDispatchPoolId()) {
            $dataArray['dispatchPoolId'] = $data->getDispatchPoolId();
        }
        if ($data->isInitialized('messageGroup') && null !== $data->getMessageGroup()) {
            $dataArray['messageGroup'] = $data->getMessageGroup();
        }
        if ($data->isInitialized('sequence') && null !== $data->getSequence()) {
            $dataArray['sequence'] = $data->getSequence();
        }
        if ($data->isInitialized('timeoutSeconds') && null !== $data->getTimeoutSeconds()) {
            $dataArray['timeoutSeconds'] = $data->getTimeoutSeconds();
        }
        if ($data->isInitialized('schemaId') && null !== $data->getSchemaId()) {
            $dataArray['schemaId'] = $data->getSchemaId();
        }
        if ($data->isInitialized('maxRetries') && null !== $data->getMaxRetries()) {
            $dataArray['maxRetries'] = $data->getMaxRetries();
        }
        if ($data->isInitialized('retryStrategy') && null !== $data->getRetryStrategy()) {
            $dataArray['retryStrategy'] = $data->getRetryStrategy();
        }
        if ($data->isInitialized('scheduledFor') && null !== $data->getScheduledFor()) {
            $dataArray['scheduledFor'] = $data->getScheduledFor()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('expiresAt') && null !== $data->getExpiresAt()) {
            $dataArray['expiresAt'] = $data->getExpiresAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('idempotencyKey') && null !== $data->getIdempotencyKey()) {
            $dataArray['idempotencyKey'] = $data->getIdempotencyKey();
        }
        if ($data->isInitialized('externalId') && null !== $data->getExternalId()) {
            $dataArray['externalId'] = $data->getExternalId();
        }
        $dataArray['queueUrl'] = $data->getQueueUrl();
        foreach ($data as $key_2 => $value_2) {
            if (preg_match('/.*/', (string) $key_2)) {
                $dataArray[$key_2] = $value_2;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\CreateDispatchJobRequest::class => false];
    }
}