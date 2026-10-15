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
class BudgetReportNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\BudgetReport::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\BudgetReport::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\BudgetReport();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('budgetBytes', $data) && $data['budgetBytes'] !== null) {
            $object->setBudgetBytes($data['budgetBytes']);
        }
        elseif (\array_key_exists('budgetBytes', $data) && $data['budgetBytes'] === null) {
            $object->setBudgetBytes(null);
        }
        if (\array_key_exists('compiledBytes', $data) && $data['compiledBytes'] !== null) {
            $object->setCompiledBytes($data['compiledBytes']);
        }
        elseif (\array_key_exists('compiledBytes', $data) && $data['compiledBytes'] === null) {
            $object->setCompiledBytes(null);
        }
        if (\array_key_exists('evictions', $data) && $data['evictions'] !== null) {
            $object->setEvictions($data['evictions']);
        }
        elseif (\array_key_exists('evictions', $data) && $data['evictions'] === null) {
            $object->setEvictions(null);
        }
        if (\array_key_exists('limitBytes', $data) && $data['limitBytes'] !== null) {
            $object->setLimitBytes($data['limitBytes']);
        }
        elseif (\array_key_exists('limitBytes', $data) && $data['limitBytes'] === null) {
            $object->setLimitBytes(null);
        }
        if (\array_key_exists('linearBytes', $data) && $data['linearBytes'] !== null) {
            $object->setLinearBytes($data['linearBytes']);
        }
        elseif (\array_key_exists('linearBytes', $data) && $data['linearBytes'] === null) {
            $object->setLinearBytes(null);
        }
        if (\array_key_exists('loaded', $data) && $data['loaded'] !== null) {
            $object->setLoaded($data['loaded']);
        }
        elseif (\array_key_exists('loaded', $data) && $data['loaded'] === null) {
            $object->setLoaded(null);
        }
        if (\array_key_exists('refused', $data) && $data['refused'] !== null) {
            $object->setRefused($data['refused']);
        }
        elseif (\array_key_exists('refused', $data) && $data['refused'] === null) {
            $object->setRefused(null);
        }
        if (\array_key_exists('reserveBytes', $data) && $data['reserveBytes'] !== null) {
            $object->setReserveBytes($data['reserveBytes']);
        }
        elseif (\array_key_exists('reserveBytes', $data) && $data['reserveBytes'] === null) {
            $object->setReserveBytes(null);
        }
        if (\array_key_exists('rssBytes', $data) && $data['rssBytes'] !== null) {
            $object->setRssBytes($data['rssBytes']);
        }
        elseif (\array_key_exists('rssBytes', $data) && $data['rssBytes'] === null) {
            $object->setRssBytes(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['budgetBytes'] = $data->getBudgetBytes();
        $dataArray['compiledBytes'] = $data->getCompiledBytes();
        $dataArray['evictions'] = $data->getEvictions();
        $dataArray['limitBytes'] = $data->getLimitBytes();
        $dataArray['linearBytes'] = $data->getLinearBytes();
        $dataArray['loaded'] = $data->getLoaded();
        $dataArray['refused'] = $data->getRefused();
        $dataArray['reserveBytes'] = $data->getReserveBytes();
        if ($data->isInitialized('rssBytes') && null !== $data->getRssBytes()) {
            $dataArray['rssBytes'] = $data->getRssBytes();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\BudgetReport::class => false];
    }
}