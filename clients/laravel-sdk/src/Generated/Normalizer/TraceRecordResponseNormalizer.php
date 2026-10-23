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
class TraceRecordResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\TraceRecordResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\TraceRecordResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\TraceRecordResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('bodiesRedacted', $data) && \is_int($data['bodiesRedacted'])) {
            $data['bodiesRedacted'] = (bool) $data['bodiesRedacted'];
        }
        if (\array_key_exists('at', $data) && $data['at'] !== null) {
            $object->setAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['at']));
        }
        elseif (\array_key_exists('at', $data) && $data['at'] === null) {
            $object->setAt(null);
        }
        if (\array_key_exists('bodiesRedacted', $data) && $data['bodiesRedacted'] !== null) {
            $object->setBodiesRedacted($data['bodiesRedacted']);
        }
        elseif (\array_key_exists('bodiesRedacted', $data) && $data['bodiesRedacted'] === null) {
            $object->setBodiesRedacted(null);
        }
        if (\array_key_exists('context', $data) && $data['context'] !== null) {
            $object->setContext($data['context']);
        }
        elseif (\array_key_exists('context', $data) && $data['context'] === null) {
            $object->setContext(null);
        }
        if (\array_key_exists('correlationId', $data) && $data['correlationId'] !== null) {
            $object->setCorrelationId($data['correlationId']);
        }
        elseif (\array_key_exists('correlationId', $data) && $data['correlationId'] === null) {
            $object->setCorrelationId(null);
        }
        if (\array_key_exists('data', $data) && $data['data'] !== null) {
            $object->setData($data['data']);
        }
        elseif (\array_key_exists('data', $data) && $data['data'] === null) {
            $object->setData(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('invocationId', $data) && $data['invocationId'] !== null) {
            $object->setInvocationId($data['invocationId']);
        }
        elseif (\array_key_exists('invocationId', $data) && $data['invocationId'] === null) {
            $object->setInvocationId(null);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
        }
        if (\array_key_exists('level', $data) && $data['level'] !== null) {
            $object->setLevel($data['level']);
        }
        elseif (\array_key_exists('level', $data) && $data['level'] === null) {
            $object->setLevel(null);
        }
        if (\array_key_exists('message', $data) && $data['message'] !== null) {
            $object->setMessage($data['message']);
        }
        elseif (\array_key_exists('message', $data) && $data['message'] === null) {
            $object->setMessage(null);
        }
        if (\array_key_exists('reference', $data) && $data['reference'] !== null) {
            $object->setReference($data['reference']);
        }
        elseif (\array_key_exists('reference', $data) && $data['reference'] === null) {
            $object->setReference(null);
        }
        if (\array_key_exists('runnerId', $data) && $data['runnerId'] !== null) {
            $object->setRunnerId($data['runnerId']);
        }
        elseif (\array_key_exists('runnerId', $data) && $data['runnerId'] === null) {
            $object->setRunnerId(null);
        }
        if (\array_key_exists('version', $data) && $data['version'] !== null) {
            $object->setVersion($data['version']);
        }
        elseif (\array_key_exists('version', $data) && $data['version'] === null) {
            $object->setVersion(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['at'] = $data->getAt()->format('Y-m-d\TH:i:sP');
        if ($data->isInitialized('bodiesRedacted') && null !== $data->getBodiesRedacted()) {
            $dataArray['bodiesRedacted'] = $data->getBodiesRedacted();
        }
        $dataArray['context'] = $data->getContext();
        $dataArray['correlationId'] = $data->getCorrelationId();
        $dataArray['data'] = $data->getData();
        $dataArray['id'] = $data->getId();
        $dataArray['invocationId'] = $data->getInvocationId();
        $dataArray['kind'] = $data->getKind();
        $dataArray['level'] = $data->getLevel();
        $dataArray['message'] = $data->getMessage();
        $dataArray['reference'] = $data->getReference();
        $dataArray['runnerId'] = $data->getRunnerId();
        $dataArray['version'] = $data->getVersion();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\TraceRecordResponse::class => false];
    }
}