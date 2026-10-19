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
class PruneVersionsResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PruneVersionsResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PruneVersionsResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PruneVersionsResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('dryRun', $data) && \is_int($data['dryRun'])) {
            $data['dryRun'] = (bool) $data['dryRun'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('artifactsDeleted', $data) && $data['artifactsDeleted'] !== null) {
            $object->setArtifactsDeleted($data['artifactsDeleted']);
        }
        elseif (\array_key_exists('artifactsDeleted', $data) && $data['artifactsDeleted'] === null) {
            $object->setArtifactsDeleted(null);
        }
        if (\array_key_exists('dryRun', $data) && $data['dryRun'] !== null) {
            $object->setDryRun($data['dryRun']);
        }
        elseif (\array_key_exists('dryRun', $data) && $data['dryRun'] === null) {
            $object->setDryRun(null);
        }
        if (\array_key_exists('keep', $data) && $data['keep'] !== null) {
            $object->setKeep($data['keep']);
        }
        elseif (\array_key_exists('keep', $data) && $data['keep'] === null) {
            $object->setKeep(null);
        }
        if (\array_key_exists('pruned', $data) && $data['pruned'] !== null) {
            $values = [];
            foreach ($data['pruned'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\PrunedVersionResponse::class, 'json', $context);
            }
            $object->setPruned($values);
        }
        elseif (\array_key_exists('pruned', $data) && $data['pruned'] === null) {
            $object->setPruned(null);
        }
        if (\array_key_exists('untagged', $data) && $data['untagged'] !== null) {
            $object->setUntagged($data['untagged']);
        }
        elseif (\array_key_exists('untagged', $data) && $data['untagged'] === null) {
            $object->setUntagged(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['artifactsDeleted'] = $data->getArtifactsDeleted();
        $dataArray['dryRun'] = $data->getDryRun();
        $dataArray['keep'] = $data->getKeep();
        $values = [];
        foreach ($data->getPruned() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['pruned'] = $values;
        $dataArray['untagged'] = $data->getUntagged();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PruneVersionsResponse::class => false];
    }
}