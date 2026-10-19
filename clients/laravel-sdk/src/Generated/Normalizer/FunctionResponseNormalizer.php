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
class FunctionResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FunctionResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FunctionResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FunctionResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('warm', $data) && \is_int($data['warm'])) {
            $data['warm'] = (bool) $data['warm'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('address', $data) && $data['address'] !== null) {
            $object->setAddress($data['address']);
        }
        elseif (\array_key_exists('address', $data) && $data['address'] === null) {
            $object->setAddress(null);
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
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('createdBy', $data) && $data['createdBy'] !== null) {
            $object->setCreatedBy($data['createdBy']);
        }
        elseif (\array_key_exists('createdBy', $data) && $data['createdBy'] === null) {
            $object->setCreatedBy(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('limits', $data) && $data['limits'] !== null) {
            $object->setLimits($this->denormalizer->denormalize($data['limits'], \FlowCatalyst\Generated\Model\LimitsDTO::class, 'json', $context));
        }
        elseif (\array_key_exists('limits', $data) && $data['limits'] === null) {
            $object->setLimits(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('pool', $data) && $data['pool'] !== null) {
            $object->setPool($data['pool']);
        }
        elseif (\array_key_exists('pool', $data) && $data['pool'] === null) {
            $object->setPool(null);
        }
        if (\array_key_exists('serviceAccountCode', $data) && $data['serviceAccountCode'] !== null) {
            $object->setServiceAccountCode($data['serviceAccountCode']);
        }
        elseif (\array_key_exists('serviceAccountCode', $data) && $data['serviceAccountCode'] === null) {
            $object->setServiceAccountCode(null);
        }
        if (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] !== null) {
            $object->setServiceAccountId($data['serviceAccountId']);
        }
        elseif (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] === null) {
            $object->setServiceAccountId(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
        }
        if (\array_key_exists('warm', $data) && $data['warm'] !== null) {
            $object->setWarm($data['warm']);
        }
        elseif (\array_key_exists('warm', $data) && $data['warm'] === null) {
            $object->setWarm(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['address'] = $data->getAddress();
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['applicationId'] = $data->getApplicationId();
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['clientId'] = $data->getClientId();
        }
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('createdBy') && null !== $data->getCreatedBy()) {
            $dataArray['createdBy'] = $data->getCreatedBy();
        }
        if ($data->isInitialized('description') && null !== $data->getDescription()) {
            $dataArray['description'] = $data->getDescription();
        }
        $dataArray['id'] = $data->getId();
        $dataArray['limits'] = $this->normalizer->normalize($data->getLimits(), 'json', $context);
        $dataArray['name'] = $data->getName();
        if ($data->isInitialized('pool') && null !== $data->getPool()) {
            $dataArray['pool'] = $data->getPool();
        }
        if ($data->isInitialized('serviceAccountCode') && null !== $data->getServiceAccountCode()) {
            $dataArray['serviceAccountCode'] = $data->getServiceAccountCode();
        }
        if ($data->isInitialized('serviceAccountId') && null !== $data->getServiceAccountId()) {
            $dataArray['serviceAccountId'] = $data->getServiceAccountId();
        }
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['warm'] = $data->getWarm();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FunctionResponse::class => false];
    }
}