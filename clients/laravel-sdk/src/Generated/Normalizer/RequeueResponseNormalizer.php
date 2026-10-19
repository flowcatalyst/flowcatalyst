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
class RequeueResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\RequeueResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\RequeueResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\RequeueResponse();
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
        if (\array_key_exists('redriven', $data) && $data['redriven'] !== null) {
            $object->setRedriven($data['redriven']);
        }
        elseif (\array_key_exists('redriven', $data) && $data['redriven'] === null) {
            $object->setRedriven(null);
        }
        if (\array_key_exists('requeued', $data) && $data['requeued'] !== null) {
            $object->setRequeued($data['requeued']);
        }
        elseif (\array_key_exists('requeued', $data) && $data['requeued'] === null) {
            $object->setRequeued(null);
        }
        if (\array_key_exists('skipped', $data) && $data['skipped'] !== null) {
            $values = [];
            foreach ($data['skipped'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\SkippedJobResponse::class, 'json', $context);
            }
            $object->setSkipped($values);
        }
        elseif (\array_key_exists('skipped', $data) && $data['skipped'] === null) {
            $object->setSkipped(null);
        }
        if (\array_key_exists('skippedCompleted', $data) && $data['skippedCompleted'] !== null) {
            $object->setSkippedCompleted($data['skippedCompleted']);
        }
        elseif (\array_key_exists('skippedCompleted', $data) && $data['skippedCompleted'] === null) {
            $object->setSkippedCompleted(null);
        }
        if (\array_key_exists('skippedInFlight', $data) && $data['skippedInFlight'] !== null) {
            $object->setSkippedInFlight($data['skippedInFlight']);
        }
        elseif (\array_key_exists('skippedInFlight', $data) && $data['skippedInFlight'] === null) {
            $object->setSkippedInFlight(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['redriven'] = $data->getRedriven();
        $dataArray['requeued'] = $data->getRequeued();
        $values = [];
        foreach ($data->getSkipped() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['skipped'] = $values;
        $dataArray['skippedCompleted'] = $data->getSkippedCompleted();
        $dataArray['skippedInFlight'] = $data->getSkippedInFlight();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\RequeueResponse::class => false];
    }
}