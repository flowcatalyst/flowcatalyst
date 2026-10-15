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
class RouteNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\Route::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\Route::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\Route();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('address', $data) && $data['address'] !== null) {
            $object->setAddress($data['address']);
        }
        elseif (\array_key_exists('address', $data) && $data['address'] === null) {
            $object->setAddress(null);
        }
        if (\array_key_exists('alias', $data) && $data['alias'] !== null) {
            $object->setAlias($data['alias']);
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
        }
        elseif (\array_key_exists('aliasPrefixes', $data) && $data['aliasPrefixes'] === null) {
            $object->setAliasPrefixes(null);
        }
        if (\array_key_exists('hostname', $data) && $data['hostname'] !== null) {
            $object->setHostname($data['hostname']);
        }
        elseif (\array_key_exists('hostname', $data) && $data['hostname'] === null) {
            $object->setHostname(null);
        }
        if (\array_key_exists('pathPrefix', $data) && $data['pathPrefix'] !== null) {
            $object->setPathPrefix($data['pathPrefix']);
        }
        elseif (\array_key_exists('pathPrefix', $data) && $data['pathPrefix'] === null) {
            $object->setPathPrefix(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['address'] = $data->getAddress();
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
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\Route::class => false];
    }
}