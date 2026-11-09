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
class SyncPushKeyRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\SyncPushKeyRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\SyncPushKeyRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\SyncPushKeyRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('alg', $data) && $data['alg'] !== null) {
            $object->setAlg($data['alg']);
        }
        elseif (\array_key_exists('alg', $data) && $data['alg'] === null) {
            $object->setAlg(null);
        }
        if (\array_key_exists('crv', $data) && $data['crv'] !== null) {
            $object->setCrv($data['crv']);
        }
        elseif (\array_key_exists('crv', $data) && $data['crv'] === null) {
            $object->setCrv(null);
        }
        if (\array_key_exists('kid', $data) && $data['kid'] !== null) {
            $object->setKid($data['kid']);
        }
        elseif (\array_key_exists('kid', $data) && $data['kid'] === null) {
            $object->setKid(null);
        }
        if (\array_key_exists('kty', $data) && $data['kty'] !== null) {
            $object->setKty($data['kty']);
        }
        elseif (\array_key_exists('kty', $data) && $data['kty'] === null) {
            $object->setKty(null);
        }
        if (\array_key_exists('use', $data) && $data['use'] !== null) {
            $object->setUse($data['use']);
        }
        elseif (\array_key_exists('use', $data) && $data['use'] === null) {
            $object->setUse(null);
        }
        if (\array_key_exists('x', $data) && $data['x'] !== null) {
            $object->setX($data['x']);
        }
        elseif (\array_key_exists('x', $data) && $data['x'] === null) {
            $object->setX(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('alg') && null !== $data->getAlg()) {
            $dataArray['alg'] = $data->getAlg();
        }
        $dataArray['crv'] = $data->getCrv();
        $dataArray['kid'] = $data->getKid();
        $dataArray['kty'] = $data->getKty();
        if ($data->isInitialized('use') && null !== $data->getUse()) {
            $dataArray['use'] = $data->getUse();
        }
        $dataArray['x'] = $data->getX();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\SyncPushKeyRequest::class => false];
    }
}