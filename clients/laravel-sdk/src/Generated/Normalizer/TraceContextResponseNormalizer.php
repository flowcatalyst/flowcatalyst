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
class TraceContextResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\TraceContextResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\TraceContextResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\TraceContextResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('builtIn', $data) && \is_int($data['builtIn'])) {
            $data['builtIn'] = (bool) $data['builtIn'];
        }
        if (\array_key_exists('indefinite', $data) && \is_int($data['indefinite'])) {
            $data['indefinite'] = (bool) $data['indefinite'];
        }
        if (\array_key_exists('live', $data) && \is_int($data['live'])) {
            $data['live'] = (bool) $data['live'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('builtIn', $data) && $data['builtIn'] !== null) {
            $object->setBuiltIn($data['builtIn']);
        }
        elseif (\array_key_exists('builtIn', $data) && $data['builtIn'] === null) {
            $object->setBuiltIn(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('enabledUntil', $data) && $data['enabledUntil'] !== null) {
            $object->setEnabledUntil(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['enabledUntil']));
        }
        elseif (\array_key_exists('enabledUntil', $data) && $data['enabledUntil'] === null) {
            $object->setEnabledUntil(null);
        }
        if (\array_key_exists('indefinite', $data) && $data['indefinite'] !== null) {
            $object->setIndefinite($data['indefinite']);
        }
        elseif (\array_key_exists('indefinite', $data) && $data['indefinite'] === null) {
            $object->setIndefinite(null);
        }
        if (\array_key_exists('live', $data) && $data['live'] !== null) {
            $object->setLive($data['live']);
        }
        elseif (\array_key_exists('live', $data) && $data['live'] === null) {
            $object->setLive(null);
        }
        if (\array_key_exists('maxMinutes', $data) && $data['maxMinutes'] !== null) {
            $object->setMaxMinutes($data['maxMinutes']);
        }
        elseif (\array_key_exists('maxMinutes', $data) && $data['maxMinutes'] === null) {
            $object->setMaxMinutes(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('sampleRate', $data) && $data['sampleRate'] !== null) {
            $object->setSampleRate($data['sampleRate']);
        }
        elseif (\array_key_exists('sampleRate', $data) && $data['sampleRate'] === null) {
            $object->setSampleRate(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['builtIn'] = $data->getBuiltIn();
        $dataArray['description'] = $data->getDescription();
        $dataArray['enabledUntil'] = $data->getEnabledUntil()?->format('Y-m-d\TH:i:sP');
        $dataArray['indefinite'] = $data->getIndefinite();
        $dataArray['live'] = $data->getLive();
        $dataArray['maxMinutes'] = $data->getMaxMinutes();
        $dataArray['name'] = $data->getName();
        $dataArray['sampleRate'] = $data->getSampleRate();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\TraceContextResponse::class => false];
    }
}