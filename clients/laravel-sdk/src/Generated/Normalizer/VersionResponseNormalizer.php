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
class VersionResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\VersionResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\VersionResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\VersionResponse();
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
        if (\array_key_exists('abi', $data) && $data['abi'] !== null) {
            $object->setAbi($data['abi']);
        }
        elseif (\array_key_exists('abi', $data) && $data['abi'] === null) {
            $object->setAbi(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('describe', $data) && $data['describe'] !== null) {
            $object->setDescribe($data['describe']);
        }
        elseif (\array_key_exists('describe', $data) && $data['describe'] === null) {
            $object->setDescribe(null);
        }
        if (\array_key_exists('digest', $data) && $data['digest'] !== null) {
            $object->setDigest($data['digest']);
        }
        elseif (\array_key_exists('digest', $data) && $data['digest'] === null) {
            $object->setDigest(null);
        }
        if (\array_key_exists('failure', $data) && $data['failure'] !== null) {
            $object->setFailure($data['failure']);
        }
        elseif (\array_key_exists('failure', $data) && $data['failure'] === null) {
            $object->setFailure(null);
        }
        if (\array_key_exists('functionId', $data) && $data['functionId'] !== null) {
            $object->setFunctionId($data['functionId']);
        }
        elseif (\array_key_exists('functionId', $data) && $data['functionId'] === null) {
            $object->setFunctionId(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('number', $data) && $data['number'] !== null) {
            $object->setNumber($data['number']);
        }
        elseif (\array_key_exists('number', $data) && $data['number'] === null) {
            $object->setNumber(null);
        }
        if (\array_key_exists('publishedBy', $data) && $data['publishedBy'] !== null) {
            $object->setPublishedBy($data['publishedBy']);
        }
        elseif (\array_key_exists('publishedBy', $data) && $data['publishedBy'] === null) {
            $object->setPublishedBy(null);
        }
        if (\array_key_exists('readyAt', $data) && $data['readyAt'] !== null) {
            $object->setReadyAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['readyAt']));
        }
        elseif (\array_key_exists('readyAt', $data) && $data['readyAt'] === null) {
            $object->setReadyAt(null);
        }
        if (\array_key_exists('runtime', $data) && $data['runtime'] !== null) {
            $object->setRuntime($data['runtime']);
        }
        elseif (\array_key_exists('runtime', $data) && $data['runtime'] === null) {
            $object->setRuntime(null);
        }
        if (\array_key_exists('sizeBytes', $data) && $data['sizeBytes'] !== null) {
            $object->setSizeBytes($data['sizeBytes']);
        }
        elseif (\array_key_exists('sizeBytes', $data) && $data['sizeBytes'] === null) {
            $object->setSizeBytes(null);
        }
        if (\array_key_exists('status', $data) && $data['status'] !== null) {
            $object->setStatus($data['status']);
        }
        elseif (\array_key_exists('status', $data) && $data['status'] === null) {
            $object->setStatus(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['abi'] = $data->getAbi();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['describe'] = $data->getDescribe();
        $dataArray['digest'] = $data->getDigest();
        if ($data->isInitialized('failure') && null !== $data->getFailure()) {
            $dataArray['failure'] = $data->getFailure();
        }
        $dataArray['functionId'] = $data->getFunctionId();
        $dataArray['id'] = $data->getId();
        $dataArray['number'] = $data->getNumber();
        if ($data->isInitialized('publishedBy') && null !== $data->getPublishedBy()) {
            $dataArray['publishedBy'] = $data->getPublishedBy();
        }
        if ($data->isInitialized('readyAt') && null !== $data->getReadyAt()) {
            $dataArray['readyAt'] = $data->getReadyAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['runtime'] = $data->getRuntime();
        $dataArray['sizeBytes'] = $data->getSizeBytes();
        $dataArray['status'] = $data->getStatus();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\VersionResponse::class => false];
    }
}