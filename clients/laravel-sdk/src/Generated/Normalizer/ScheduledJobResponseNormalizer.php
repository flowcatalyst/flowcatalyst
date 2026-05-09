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
class ScheduledJobResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ScheduledJobResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ScheduledJobResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ScheduledJobResponse();
        if (\array_key_exists('concurrent', $data) && \is_int($data['concurrent'])) {
            $data['concurrent'] = (bool) $data['concurrent'];
        }
        if (\array_key_exists('hasActiveInstance', $data) && \is_int($data['hasActiveInstance'])) {
            $data['hasActiveInstance'] = (bool) $data['hasActiveInstance'];
        }
        if (\array_key_exists('tracksCompletion', $data) && \is_int($data['tracksCompletion'])) {
            $data['tracksCompletion'] = (bool) $data['tracksCompletion'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
            unset($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
            unset($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('concurrent', $data) && $data['concurrent'] !== null) {
            $object->setConcurrent($data['concurrent']);
            unset($data['concurrent']);
        }
        elseif (\array_key_exists('concurrent', $data) && $data['concurrent'] === null) {
            $object->setConcurrent(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('createdBy', $data) && $data['createdBy'] !== null) {
            $object->setCreatedBy($data['createdBy']);
            unset($data['createdBy']);
        }
        elseif (\array_key_exists('createdBy', $data) && $data['createdBy'] === null) {
            $object->setCreatedBy(null);
        }
        if (\array_key_exists('crons', $data) && $data['crons'] !== null) {
            $values = [];
            foreach ($data['crons'] as $value) {
                $values[] = $value;
            }
            $object->setCrons($values);
            unset($data['crons']);
        }
        elseif (\array_key_exists('crons', $data) && $data['crons'] === null) {
            $object->setCrons(null);
        }
        if (\array_key_exists('deliveryMaxAttempts', $data) && $data['deliveryMaxAttempts'] !== null) {
            $object->setDeliveryMaxAttempts($data['deliveryMaxAttempts']);
            unset($data['deliveryMaxAttempts']);
        }
        elseif (\array_key_exists('deliveryMaxAttempts', $data) && $data['deliveryMaxAttempts'] === null) {
            $object->setDeliveryMaxAttempts(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
            unset($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('hasActiveInstance', $data) && $data['hasActiveInstance'] !== null) {
            $object->setHasActiveInstance($data['hasActiveInstance']);
            unset($data['hasActiveInstance']);
        }
        elseif (\array_key_exists('hasActiveInstance', $data) && $data['hasActiveInstance'] === null) {
            $object->setHasActiveInstance(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
            unset($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('lastFiredAt', $data) && $data['lastFiredAt'] !== null) {
            $object->setLastFiredAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['lastFiredAt']));
            unset($data['lastFiredAt']);
        }
        elseif (\array_key_exists('lastFiredAt', $data) && $data['lastFiredAt'] === null) {
            $object->setLastFiredAt(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
            unset($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('payload', $data) && $data['payload'] !== null) {
            $object->setPayload($data['payload']);
            unset($data['payload']);
        }
        elseif (\array_key_exists('payload', $data) && $data['payload'] === null) {
            $object->setPayload(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
            unset($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('targetUrl', $data) && $data['targetUrl'] !== null) {
            $object->setTargetUrl($data['targetUrl']);
            unset($data['targetUrl']);
        }
        elseif (\array_key_exists('targetUrl', $data) && $data['targetUrl'] === null) {
            $object->setTargetUrl(null);
        }
        if (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] !== null) {
            $object->setTimeoutSeconds($data['timeoutSeconds']);
            unset($data['timeoutSeconds']);
        }
        elseif (\array_key_exists('timeoutSeconds', $data) && $data['timeoutSeconds'] === null) {
            $object->setTimeoutSeconds(null);
        }
        if (\array_key_exists('timezone', $data) && $data['timezone'] !== null) {
            $object->setTimezone($data['timezone']);
            unset($data['timezone']);
        }
        elseif (\array_key_exists('timezone', $data) && $data['timezone'] === null) {
            $object->setTimezone(null);
        }
        if (\array_key_exists('tracksCompletion', $data) && $data['tracksCompletion'] !== null) {
            $object->setTracksCompletion($data['tracksCompletion']);
            unset($data['tracksCompletion']);
        }
        elseif (\array_key_exists('tracksCompletion', $data) && $data['tracksCompletion'] === null) {
            $object->setTracksCompletion(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
            unset($data['updatedAt']);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        if (\array_key_exists('updatedBy', $data) && $data['updatedBy'] !== null) {
            $object->setUpdatedBy($data['updatedBy']);
            unset($data['updatedBy']);
        }
        elseif (\array_key_exists('updatedBy', $data) && $data['updatedBy'] === null) {
            $object->setUpdatedBy(null);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
            unset($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
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
        if ($data->isInitialized('clientId')) {
            $dataArray['clientId'] = $data->getClientId();
        }
        $dataArray['code'] = $data->getCode();
        $dataArray['concurrent'] = $data->getConcurrent();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('createdBy')) {
            $dataArray['createdBy'] = $data->getCreatedBy();
        }
        $values = [];
        foreach ($data->getCrons() as $value) {
            $values[] = $value;
        }
        $dataArray['crons'] = $values;
        $dataArray['deliveryMaxAttempts'] = $data->getDeliveryMaxAttempts();
        if ($data->isInitialized('description')) {
            $dataArray['description'] = $data->getDescription();
        }
        $dataArray['hasActiveInstance'] = $data->getHasActiveInstance();
        $dataArray['id'] = $data->getId();
        if ($data->isInitialized('lastFiredAt')) {
            $dataArray['lastFiredAt'] = $data->getLastFiredAt()?->format('Y-m-d\TH:i:sP');
        }
        $dataArray['name'] = $data->getName();
        if ($data->isInitialized('payload') && null !== $data->getPayload()) {
            $dataArray['payload'] = $data->getPayload();
        }
        $dataArray['status'] = $data->getStatus();
        if ($data->isInitialized('targetUrl')) {
            $dataArray['targetUrl'] = $data->getTargetUrl();
        }
        if ($data->isInitialized('timeoutSeconds')) {
            $dataArray['timeoutSeconds'] = $data->getTimeoutSeconds();
        }
        $dataArray['timezone'] = $data->getTimezone();
        $dataArray['tracksCompletion'] = $data->getTracksCompletion();
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('updatedBy')) {
            $dataArray['updatedBy'] = $data->getUpdatedBy();
        }
        $dataArray['version'] = $data->getVersion();
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ScheduledJobResponse::class => false];
    }
}