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
class PutRouteRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PutRouteRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PutRouteRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\PutRouteRequest();
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
        if (\array_key_exists('alias', $data) && $data['alias'] !== null) {
            $object->setAlias($data['alias']);
            unset($data['alias']);
        }
        elseif (\array_key_exists('alias', $data) && $data['alias'] === null) {
            $object->setAlias(null);
        }
        if (\array_key_exists('aliasPrefixes', $data) && $data['aliasPrefixes'] !== null) {
            $values = [];
            foreach ($data['aliasPrefixes'] as $value) {
                $values[] = $value;
            }
            $object->setAliasPrefixes($values);
            unset($data['aliasPrefixes']);
        }
        elseif (\array_key_exists('aliasPrefixes', $data) && $data['aliasPrefixes'] === null) {
            $object->setAliasPrefixes(null);
        }
        if (\array_key_exists('hostname', $data) && $data['hostname'] !== null) {
            $object->setHostname($data['hostname']);
            unset($data['hostname']);
        }
        elseif (\array_key_exists('hostname', $data) && $data['hostname'] === null) {
            $object->setHostname(null);
        }
        if (\array_key_exists('pathPrefix', $data) && $data['pathPrefix'] !== null) {
            $object->setPathPrefix($data['pathPrefix']);
            unset($data['pathPrefix']);
        }
        elseif (\array_key_exists('pathPrefix', $data) && $data['pathPrefix'] === null) {
            $object->setPathPrefix(null);
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
        if ($data->isInitialized('alias') && null !== $data->getAlias()) {
            $dataArray['alias'] = $data->getAlias();
        }
        if ($data->isInitialized('aliasPrefixes') && null !== $data->getAliasPrefixes()) {
            $values = [];
            foreach ($data->getAliasPrefixes() as $value) {
                $values[] = $value;
            }
            $dataArray['aliasPrefixes'] = $values;
        }
        $dataArray['hostname'] = $data->getHostname();
        $dataArray['pathPrefix'] = $data->getPathPrefix();
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PutRouteRequest::class => false];
    }
}