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
class InFlightMessagesResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\InFlightMessagesResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\InFlightMessagesResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\InFlightMessagesResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('byMessageGroup', $data) && $data['byMessageGroup'] !== null) {
            $values = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['byMessageGroup'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setByMessageGroup($values);
            unset($data['byMessageGroup']);
        }
        elseif (\array_key_exists('byMessageGroup', $data) && $data['byMessageGroup'] === null) {
            $object->setByMessageGroup(null);
        }
        if (\array_key_exists('byPool', $data) && $data['byPool'] !== null) {
            $values_1 = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['byPool'] as $key_1 => $value_1) {
                $values_1[$key_1] = $value_1;
            }
            $object->setByPool($values_1);
            unset($data['byPool']);
        }
        elseif (\array_key_exists('byPool', $data) && $data['byPool'] === null) {
            $object->setByPool(null);
        }
        if (\array_key_exists('messages', $data) && $data['messages'] !== null) {
            $values_2 = [];
            foreach ($data['messages'] as $value_2) {
                $values_2[] = $this->denormalizer->denormalize($value_2, \FlowCatalyst\Generated\Model\InFlightMessage::class, 'json', $context);
            }
            $object->setMessages($values_2);
            unset($data['messages']);
        }
        elseif (\array_key_exists('messages', $data) && $data['messages'] === null) {
            $object->setMessages(null);
        }
        if (\array_key_exists('totalInFlight', $data) && $data['totalInFlight'] !== null) {
            $object->setTotalInFlight($data['totalInFlight']);
            unset($data['totalInFlight']);
        }
        elseif (\array_key_exists('totalInFlight', $data) && $data['totalInFlight'] === null) {
            $object->setTotalInFlight(null);
        }
        foreach ($data as $key_2 => $value_3) {
            if (preg_match('/.*/', (string) $key_2)) {
                $object[$key_2] = $value_3;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getByMessageGroup() as $key => $value) {
            $values[$key] = $value;
        }
        $dataArray['byMessageGroup'] = $values;
        $values_1 = [];
        foreach ($data->getByPool() as $key_1 => $value_1) {
            $values_1[$key_1] = $value_1;
        }
        $dataArray['byPool'] = $values_1;
        $values_2 = [];
        foreach ($data->getMessages() as $value_2) {
            $values_2[] = $this->normalizer->normalize($value_2, 'json', $context);
        }
        $dataArray['messages'] = $values_2;
        $dataArray['totalInFlight'] = $data->getTotalInFlight();
        foreach ($data as $key_2 => $value_3) {
            if (preg_match('/.*/', (string) $key_2)) {
                $dataArray[$key_2] = $value_3;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\InFlightMessagesResponse::class => false];
    }
}