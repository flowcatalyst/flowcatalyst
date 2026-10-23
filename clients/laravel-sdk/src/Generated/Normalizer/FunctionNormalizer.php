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
class FunctionNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\_Function::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\_Function::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\_Function();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('warm', $data) && \is_int($data['warm'])) {
            $data['warm'] = (bool) $data['warm'];
        }
        if (\array_key_exists('address', $data) && $data['address'] !== null) {
            $object->setAddress($data['address']);
        }
        elseif (\array_key_exists('address', $data) && $data['address'] === null) {
            $object->setAddress(null);
        }
        if (\array_key_exists('applicationId', $data) && $data['applicationId'] !== null) {
            $object->setApplicationId($data['applicationId']);
        }
        elseif (\array_key_exists('applicationId', $data) && $data['applicationId'] === null) {
            $object->setApplicationId(null);
        }
        if (\array_key_exists('clientId', $data) && $data['clientId'] !== null) {
            $object->setClientId($data['clientId']);
        }
        elseif (\array_key_exists('clientId', $data) && $data['clientId'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('config', $data) && $data['config'] !== null) {
            $values = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['config'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setConfig($values);
        }
        elseif (\array_key_exists('config', $data) && $data['config'] === null) {
            $object->setConfig(null);
        }
        if (\array_key_exists('db', $data) && $data['db'] !== null) {
            $values_1 = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['db'] as $key_1 => $value_1) {
                $values_1[$key_1] = $value_1;
            }
            $object->setDb($values_1);
        }
        elseif (\array_key_exists('db', $data) && $data['db'] === null) {
            $object->setDb(null);
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('limits', $data) && $data['limits'] !== null) {
            $object->setLimits($this->denormalizer->denormalize($data['limits'], \FlowCatalyst\Generated\Model\Limits::class, 'json', $context));
        }
        elseif (\array_key_exists('limits', $data) && $data['limits'] === null) {
            $object->setLimits(null);
        }
        if (\array_key_exists('secrets', $data) && $data['secrets'] !== null) {
            $values_2 = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['secrets'] as $key_2 => $value_2) {
                $values_2[$key_2] = $value_2;
            }
            $object->setSecrets($values_2);
        }
        elseif (\array_key_exists('secrets', $data) && $data['secrets'] === null) {
            $object->setSecrets(null);
        }
        if (\array_key_exists('trace', $data) && $data['trace'] !== null) {
            $values_3 = [];
            foreach ($data['trace'] as $value_3) {
                $values_3[] = $this->denormalizer->denormalize($value_3, \FlowCatalyst\Generated\Model\TraceState::class, 'json', $context);
            }
            $object->setTrace($values_3);
        }
        elseif (\array_key_exists('trace', $data) && $data['trace'] === null) {
            $object->setTrace(null);
        }
        if (\array_key_exists('versions', $data) && $data['versions'] !== null) {
            $values_4 = [];
            foreach ($data['versions'] as $value_4) {
                $values_4[] = $this->denormalizer->denormalize($value_4, \FlowCatalyst\Generated\Model\Version::class, 'json', $context);
            }
            $object->setVersions($values_4);
        }
        elseif (\array_key_exists('versions', $data) && $data['versions'] === null) {
            $object->setVersions(null);
        }
        if (\array_key_exists('warm', $data) && $data['warm'] !== null) {
            $object->setWarm($data['warm']);
        }
        elseif (\array_key_exists('warm', $data) && $data['warm'] === null) {
            $object->setWarm(null);
        }
        if (\array_key_exists('webhookSecret', $data) && $data['webhookSecret'] !== null) {
            $object->setWebhookSecret($data['webhookSecret']);
        }
        elseif (\array_key_exists('webhookSecret', $data) && $data['webhookSecret'] === null) {
            $object->setWebhookSecret(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['address'] = $data->getAddress();
        $dataArray['applicationId'] = $data->getApplicationId();
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['clientId'] = $data->getClientId();
        }
        if ($data->isInitialized('config') && null !== $data->getConfig()) {
            $values = [];
            foreach ($data->getConfig() as $key => $value) {
                $values[$key] = $value;
            }
            $dataArray['config'] = $values;
        }
        if ($data->isInitialized('db') && null !== $data->getDb()) {
            $values_1 = [];
            foreach ($data->getDb() as $key_1 => $value_1) {
                $values_1[$key_1] = $value_1;
            }
            $dataArray['db'] = $values_1;
        }
        $dataArray['id'] = $data->getId();
        $dataArray['limits'] = $this->normalizer->normalize($data->getLimits(), 'json', $context);
        if ($data->isInitialized('secrets') && null !== $data->getSecrets()) {
            $values_2 = [];
            foreach ($data->getSecrets() as $key_2 => $value_2) {
                $values_2[$key_2] = $value_2;
            }
            $dataArray['secrets'] = $values_2;
        }
        if ($data->isInitialized('trace') && null !== $data->getTrace()) {
            $values_3 = [];
            foreach ($data->getTrace() as $value_3) {
                $values_3[] = $this->normalizer->normalize($value_3, 'json', $context);
            }
            $dataArray['trace'] = $values_3;
        }
        $values_4 = [];
        foreach ($data->getVersions() as $value_4) {
            $values_4[] = $this->normalizer->normalize($value_4, 'json', $context);
        }
        $dataArray['versions'] = $values_4;
        $dataArray['warm'] = $data->getWarm();
        if ($data->isInitialized('webhookSecret') && null !== $data->getWebhookSecret()) {
            $dataArray['webhookSecret'] = $data->getWebhookSecret();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\_Function::class => false];
    }
}