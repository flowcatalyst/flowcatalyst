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
class HeartbeatNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\Heartbeat::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\Heartbeat::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\Heartbeat();
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
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('budget', $data) && $data['budget'] !== null) {
            $object->setBudget($this->denormalizer->denormalize($data['budget'], \FlowCatalyst\Generated\Model\BudgetReport::class, 'json', $context));
            unset($data['budget']);
        }
        elseif (\array_key_exists('budget', $data) && $data['budget'] === null) {
            $object->setBudget(null);
        }
        if (\array_key_exists('capabilities', $data) && $data['capabilities'] !== null) {
            $object->setCapabilities($this->denormalizer->denormalize($data['capabilities'], \FlowCatalyst\Generated\Model\RunnerCapabilities::class, 'json', $context));
            unset($data['capabilities']);
        }
        elseif (\array_key_exists('capabilities', $data) && $data['capabilities'] === null) {
            $object->setCapabilities(null);
        }
        if (\array_key_exists('pool', $data) && $data['pool'] !== null) {
            $object->setPool($data['pool']);
            unset($data['pool']);
        }
        elseif (\array_key_exists('pool', $data) && $data['pool'] === null) {
            $object->setPool(null);
        }
        if (\array_key_exists('revision', $data) && $data['revision'] !== null) {
            $object->setRevision($data['revision']);
            unset($data['revision']);
        }
        elseif (\array_key_exists('revision', $data) && $data['revision'] === null) {
            $object->setRevision(null);
        }
        if (\array_key_exists('runnerId', $data) && $data['runnerId'] !== null) {
            $object->setRunnerId($data['runnerId']);
            unset($data['runnerId']);
        }
        elseif (\array_key_exists('runnerId', $data) && $data['runnerId'] === null) {
            $object->setRunnerId(null);
        }
        if (\array_key_exists('startedAt', $data) && $data['startedAt'] !== null) {
            $object->setStartedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['startedAt']));
            unset($data['startedAt']);
        }
        elseif (\array_key_exists('startedAt', $data) && $data['startedAt'] === null) {
            $object->setStartedAt(null);
        }
        if (\array_key_exists('versions', $data) && $data['versions'] !== null) {
            $values = [];
            foreach ($data['versions'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\VersionReport::class, 'json', $context);
            }
            $object->setVersions($values);
            unset($data['versions']);
        }
        elseif (\array_key_exists('versions', $data) && $data['versions'] === null) {
            $object->setVersions(null);
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
        $dataArray['budget'] = $this->normalizer->normalize($data->getBudget(), 'json', $context);
        if ($data->isInitialized('capabilities') && null !== $data->getCapabilities()) {
            $dataArray['capabilities'] = $this->normalizer->normalize($data->getCapabilities(), 'json', $context);
        }
        $dataArray['pool'] = $data->getPool();
        $dataArray['revision'] = $data->getRevision();
        $dataArray['runnerId'] = $data->getRunnerId();
        $dataArray['startedAt'] = $data->getStartedAt()->format('Y-m-d\TH:i:sP');
        $values = [];
        foreach ($data->getVersions() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['versions'] = $values;
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\Heartbeat::class => false];
    }
}