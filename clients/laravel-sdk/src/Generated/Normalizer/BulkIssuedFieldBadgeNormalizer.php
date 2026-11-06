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
class BulkIssuedFieldBadgeNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\BulkIssuedFieldBadge::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\BulkIssuedFieldBadge::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\BulkIssuedFieldBadge();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('badge', $data) && $data['badge'] !== null) {
            $object->setBadge($this->denormalizer->denormalize($data['badge'], \FlowCatalyst\Generated\Model\FieldBadgeResponse::class, 'json', $context));
        }
        elseif (\array_key_exists('badge', $data) && $data['badge'] === null) {
            $object->setBadge(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('token', $data) && $data['token'] !== null) {
            $object->setToken($data['token']);
        }
        elseif (\array_key_exists('token', $data) && $data['token'] === null) {
            $object->setToken(null);
        }
        if (\array_key_exists('workerCode', $data) && $data['workerCode'] !== null) {
            $object->setWorkerCode($data['workerCode']);
        }
        elseif (\array_key_exists('workerCode', $data) && $data['workerCode'] === null) {
            $object->setWorkerCode(null);
        }
        if (\array_key_exists('workerId', $data) && $data['workerId'] !== null) {
            $object->setWorkerId($data['workerId']);
        }
        elseif (\array_key_exists('workerId', $data) && $data['workerId'] === null) {
            $object->setWorkerId(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['badge'] = $this->normalizer->normalize($data->getBadge(), 'json', $context);
        $dataArray['name'] = $data->getName();
        $dataArray['token'] = $data->getToken();
        $dataArray['workerCode'] = $data->getWorkerCode();
        $dataArray['workerId'] = $data->getWorkerId();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\BulkIssuedFieldBadge::class => false];
    }
}