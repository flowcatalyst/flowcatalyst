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
class PlatformStatsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\PlatformStats::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\PlatformStats::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\PlatformStats();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('eventTypes', $data) && $data['eventTypes'] !== null) {
            $object->setEventTypes($data['eventTypes']);
            unset($data['eventTypes']);
        }
        elseif (\array_key_exists('eventTypes', $data) && $data['eventTypes'] === null) {
            $object->setEventTypes(null);
        }
        if (\array_key_exists('subscriptions', $data) && $data['subscriptions'] !== null) {
            $object->setSubscriptions($data['subscriptions']);
            unset($data['subscriptions']);
        }
        elseif (\array_key_exists('subscriptions', $data) && $data['subscriptions'] === null) {
            $object->setSubscriptions(null);
        }
        if (\array_key_exists('messagesProcessed', $data) && $data['messagesProcessed'] !== null) {
            $object->setMessagesProcessed($data['messagesProcessed']);
            unset($data['messagesProcessed']);
        }
        elseif (\array_key_exists('messagesProcessed', $data) && $data['messagesProcessed'] === null) {
            $object->setMessagesProcessed(null);
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
        if ($data->isInitialized('eventTypes') && null !== $data->getEventTypes()) {
            $dataArray['eventTypes'] = $data->getEventTypes();
        }
        if ($data->isInitialized('subscriptions') && null !== $data->getSubscriptions()) {
            $dataArray['subscriptions'] = $data->getSubscriptions();
        }
        if ($data->isInitialized('messagesProcessed') && null !== $data->getMessagesProcessed()) {
            $dataArray['messagesProcessed'] = $data->getMessagesProcessed();
        }
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\PlatformStats::class => false];
    }
}