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
class MatchEmailsResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\MatchEmailsResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\MatchEmailsResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\MatchEmailsResponse();
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
        }
        elseif (\array_key_exists('$schema', $data) && $data['$schema'] === null) {
            $object->setDollarSchema(null);
        }
        if (\array_key_exists('invalid', $data) && $data['invalid'] !== null) {
            $values = [];
            foreach ($data['invalid'] as $value) {
                $values[] = $value;
            }
            $object->setInvalid($values);
        }
        elseif (\array_key_exists('invalid', $data) && $data['invalid'] === null) {
            $object->setInvalid(null);
        }
        if (\array_key_exists('matches', $data) && $data['matches'] !== null) {
            $values_1 = [];
            foreach ($data['matches'] as $value_1) {
                $values_1[] = $this->denormalizer->denormalize($value_1, \FlowCatalyst\Generated\Model\PrincipalResponse::class, 'json', $context);
            }
            $object->setMatches($values_1);
        }
        elseif (\array_key_exists('matches', $data) && $data['matches'] === null) {
            $object->setMatches(null);
        }
        if (\array_key_exists('unmatched', $data) && $data['unmatched'] !== null) {
            $values_2 = [];
            foreach ($data['unmatched'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setUnmatched($values_2);
        }
        elseif (\array_key_exists('unmatched', $data) && $data['unmatched'] === null) {
            $object->setUnmatched(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getInvalid() as $value) {
            $values[] = $value;
        }
        $dataArray['invalid'] = $values;
        $values_1 = [];
        foreach ($data->getMatches() as $value_1) {
            $values_1[] = $this->normalizer->normalize($value_1, 'json', $context);
        }
        $dataArray['matches'] = $values_1;
        $values_2 = [];
        foreach ($data->getUnmatched() as $value_2) {
            $values_2[] = $value_2;
        }
        $dataArray['unmatched'] = $values_2;
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\MatchEmailsResponse::class => false];
    }
}