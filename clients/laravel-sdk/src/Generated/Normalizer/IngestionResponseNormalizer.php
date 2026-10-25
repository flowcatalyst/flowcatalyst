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
class IngestionResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\IngestionResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\IngestionResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\IngestionResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('keepFailedPayloads', $data) && \is_int($data['keepFailedPayloads'])) {
            $data['keepFailedPayloads'] = (bool) $data['keepFailedPayloads'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('applicationId', $data) && $data['applicationId'] !== null) {
            $object->setApplicationId($data['applicationId']);
        }
        elseif (\array_key_exists('applicationId', $data) && $data['applicationId'] === null) {
            $object->setApplicationId(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('clientIdentifier', $data) && $data['clientIdentifier'] !== null) {
            $object->setClientIdentifier($data['clientIdentifier']);
        }
        elseif (\array_key_exists('clientIdentifier', $data) && $data['clientIdentifier'] === null) {
            $object->setClientIdentifier(null);
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('keepFailedPayloads', $data) && $data['keepFailedPayloads'] !== null) {
            $object->setKeepFailedPayloads($data['keepFailedPayloads']);
        }
        elseif (\array_key_exists('keepFailedPayloads', $data) && $data['keepFailedPayloads'] === null) {
            $object->setKeepFailedPayloads(null);
        }
        if (\array_key_exists('latestNotice', $data) && $data['latestNotice'] !== null) {
            $object->setLatestNotice($data['latestNotice']);
        }
        elseif (\array_key_exists('latestNotice', $data) && $data['latestNotice'] === null) {
            $object->setLatestNotice(null);
        }
        if (\array_key_exists('latestNoticeAt', $data) && $data['latestNoticeAt'] !== null) {
            $object->setLatestNoticeAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['latestNoticeAt']));
        }
        elseif (\array_key_exists('latestNoticeAt', $data) && $data['latestNoticeAt'] === null) {
            $object->setLatestNoticeAt(null);
        }
        if (\array_key_exists('latestVersion', $data) && $data['latestVersion'] !== null) {
            $object->setLatestVersion($data['latestVersion']);
        }
        elseif (\array_key_exists('latestVersion', $data) && $data['latestVersion'] === null) {
            $object->setLatestVersion(null);
        }
        if (\array_key_exists('managedBy', $data) && $data['managedBy'] !== null) {
            $object->setManagedBy($data['managedBy']);
        }
        elseif (\array_key_exists('managedBy', $data) && $data['managedBy'] === null) {
            $object->setManagedBy(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('notices', $data) && $data['notices'] !== null) {
            $object->setNotices($data['notices']);
        }
        elseif (\array_key_exists('notices', $data) && $data['notices'] === null) {
            $object->setNotices(null);
        }
        if (\array_key_exists('runtimeNotice', $data) && $data['runtimeNotice'] !== null) {
            $object->setRuntimeNotice($data['runtimeNotice']);
        }
        elseif (\array_key_exists('runtimeNotice', $data) && $data['runtimeNotice'] === null) {
            $object->setRuntimeNotice(null);
        }
        if (\array_key_exists('save', $data) && $data['save'] !== null) {
            $object->setSave($this->denormalizer->denormalize($data['save'], \FlowCatalyst\Generated\Model\IngestionSaveOutcome::class, 'json', $context));
        }
        elseif (\array_key_exists('save', $data) && $data['save'] === null) {
            $object->setSave(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('split', $data) && $data['split'] !== null) {
            $object->setSplit($data['split']);
        }
        elseif (\array_key_exists('split', $data) && $data['split'] === null) {
            $object->setSplit(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        if (\array_key_exists('target', $data) && $data['target'] !== null) {
            $object->setTarget($this->denormalizer->denormalize($data['target'], \FlowCatalyst\Generated\Model\Target::class, 'json', $context));
        }
        elseif (\array_key_exists('target', $data) && $data['target'] === null) {
            $object->setTarget(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
        }
        if (\array_key_exists('warnings', $data) && $data['warnings'] !== null) {
            $values = [];
            foreach ($data['warnings'] as $value) {
                $values[] = $value;
            }
            $object->setWarnings($values);
        }
        elseif (\array_key_exists('warnings', $data) && $data['warnings'] === null) {
            $object->setWarnings(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['applicationId'] = $data->getApplicationId();
        $dataArray['clientId'] = $data->getClientId();
        $dataArray['clientIdentifier'] = $data->getClientIdentifier();
        $dataArray['code'] = $data->getCode();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['id'] = $data->getId();
        if ($data->isInitialized('keepFailedPayloads') && null !== $data->getKeepFailedPayloads()) {
            $dataArray['keepFailedPayloads'] = $data->getKeepFailedPayloads();
        }
        if ($data->isInitialized('latestNotice') && null !== $data->getLatestNotice()) {
            $dataArray['latestNotice'] = $data->getLatestNotice();
        }
        if ($data->isInitialized('latestNoticeAt') && null !== $data->getLatestNoticeAt()) {
            $dataArray['latestNoticeAt'] = $data->getLatestNoticeAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['latestVersion'] = $data->getLatestVersion();
        if ($data->isInitialized('managedBy') && null !== $data->getManagedBy()) {
            $dataArray['managedBy'] = $data->getManagedBy();
        }
        $dataArray['name'] = $data->getName();
        $dataArray['notices'] = $data->getNotices();
        if ($data->isInitialized('runtimeNotice') && null !== $data->getRuntimeNotice()) {
            $dataArray['runtimeNotice'] = $data->getRuntimeNotice();
        }
        if ($data->isInitialized('save') && null !== $data->getSave()) {
            $dataArray['save'] = $this->normalizer->normalize($data->getSave(), 'json', $context);
        }
        $dataArray['source'] = $data->getSource();
        $dataArray['split'] = $data->getSplit();
        $dataArray['status'] = $data->getStatus();
        $dataArray['target'] = $this->normalizer->normalize($data->getTarget(), 'json', $context);
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['version'] = $data->getVersion();
        if ($data->isInitialized('warnings') && null !== $data->getWarnings()) {
            $values = [];
            foreach ($data->getWarnings() as $value) {
                $values[] = $value;
            }
            $dataArray['warnings'] = $values;
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\IngestionResponse::class => false];
    }
}