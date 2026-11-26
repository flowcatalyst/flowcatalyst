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
class ProvisionServiceAccountRequestNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ProvisionServiceAccountRequest::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ProvisionServiceAccountRequest::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\ProvisionServiceAccountRequest();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('publishCredentials', $data) && \is_int($data['publishCredentials'])) {
            $data['publishCredentials'] = (bool) $data['publishCredentials'];
        }
        if (\array_key_exists('$schema', $data) && $data['$schema'] !== null) {
            $object->setDollarSchema($data['$schema']);
            unset($data['$schema']);
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
            unset($data['$schema']);
        }
        if (\array_key_exists('publishBasePath', $data) && $data['publishBasePath'] !== null) {
            $object->setPublishBasePath($data['publishBasePath']);
            unset($data['publishBasePath']);
        }
        elseif (\array_key_exists('publishBasePath', $data) && $data['publishBasePath'] === null) {
            $object->setPublishBasePath(null);
            unset($data['publishBasePath']);
        }
        if (\array_key_exists('publishCredentials', $data) && $data['publishCredentials'] !== null) {
            $object->setPublishCredentials($data['publishCredentials']);
            unset($data['publishCredentials']);
        }
        elseif (\array_key_exists('publishCredentials', $data) && $data['publishCredentials'] === null) {
            $object->setPublishCredentials(null);
            unset($data['publishCredentials']);
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('publishBasePath') && null !== $data->getPublishBasePath()) {
            $dataArray['publishBasePath'] = $data->getPublishBasePath();
        }
        if ($data->isInitialized('publishCredentials') && null !== $data->getPublishCredentials()) {
            $dataArray['publishCredentials'] = $data->getPublishCredentials();
        }
        foreach ($data->additionalPropertyEntries() as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ProvisionServiceAccountRequest::class => false];
    }
}