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
class TargetNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\Target::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\Target::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\Target();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('dataOnly', $data) && \is_int($data['dataOnly'])) {
            $data['dataOnly'] = (bool) $data['dataOnly'];
        }
        if (\array_key_exists('wrapNonJson', $data) && \is_int($data['wrapNonJson'])) {
            $data['wrapNonJson'] = (bool) $data['wrapNonJson'];
        }
        if (\array_key_exists('batchSize', $data) && $data['batchSize'] !== null) {
            $object->setBatchSize($data['batchSize']);
        }
        elseif (\array_key_exists('batchSize', $data) && $data['batchSize'] === null) {
            $object->setBatchSize(null);
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('connection', $data) && $data['connection'] !== null) {
            $object->setConnection($data['connection']);
        }
        elseif (\array_key_exists('connection', $data) && $data['connection'] === null) {
            $object->setConnection(null);
        }
        if (\array_key_exists('dataOnly', $data) && $data['dataOnly'] !== null) {
            $object->setDataOnly($data['dataOnly']);
        }
        elseif (\array_key_exists('dataOnly', $data) && $data['dataOnly'] === null) {
            $object->setDataOnly(null);
        }
        if (\array_key_exists('kind', $data) && $data['kind'] !== null) {
            $object->setKind($data['kind']);
        }
        elseif (\array_key_exists('kind', $data) && $data['kind'] === null) {
            $object->setKind(null);
        }
        if (\array_key_exists('mode', $data) && $data['mode'] !== null) {
            $object->setMode($data['mode']);
        }
        elseif (\array_key_exists('mode', $data) && $data['mode'] === null) {
            $object->setMode(null);
        }
        if (\array_key_exists('pool', $data) && $data['pool'] !== null) {
            $object->setPool($data['pool']);
        }
        elseif (\array_key_exists('pool', $data) && $data['pool'] === null) {
            $object->setPool(null);
        }
        if (\array_key_exists('serviceAccount', $data) && $data['serviceAccount'] !== null) {
            $object->setServiceAccount($data['serviceAccount']);
        }
        elseif (\array_key_exists('serviceAccount', $data) && $data['serviceAccount'] === null) {
            $object->setServiceAccount(null);
        }
        if (\array_key_exists('targetUrl', $data) && $data['targetUrl'] !== null) {
            $object->setTargetUrl($data['targetUrl']);
        }
        elseif (\array_key_exists('targetUrl', $data) && $data['targetUrl'] === null) {
            $object->setTargetUrl(null);
        }
        if (\array_key_exists('wrapNonJson', $data) && $data['wrapNonJson'] !== null) {
            $object->setWrapNonJson($data['wrapNonJson']);
        }
        elseif (\array_key_exists('wrapNonJson', $data) && $data['wrapNonJson'] === null) {
            $object->setWrapNonJson(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['batchSize'] = $data->getBatchSize();
        $dataArray['code'] = $data->getCode();
        if ($data->isInitialized('connection') && null !== $data->getConnection()) {
            $dataArray['connection'] = $data->getConnection();
        }
        if ($data->isInitialized('dataOnly') && null !== $data->getDataOnly()) {
            $dataArray['dataOnly'] = $data->getDataOnly();
        }
        if ($data->isInitialized('kind') && null !== $data->getKind()) {
            $dataArray['kind'] = $data->getKind();
        }
        if ($data->isInitialized('mode') && null !== $data->getMode()) {
            $dataArray['mode'] = $data->getMode();
        }
        if ($data->isInitialized('pool') && null !== $data->getPool()) {
            $dataArray['pool'] = $data->getPool();
        }
        if ($data->isInitialized('serviceAccount') && null !== $data->getServiceAccount()) {
            $dataArray['serviceAccount'] = $data->getServiceAccount();
        }
        if ($data->isInitialized('targetUrl') && null !== $data->getTargetUrl()) {
            $dataArray['targetUrl'] = $data->getTargetUrl();
        }
        if ($data->isInitialized('wrapNonJson') && null !== $data->getWrapNonJson()) {
            $dataArray['wrapNonJson'] = $data->getWrapNonJson();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\Target::class => false];
    }
}