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
class ItemResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ItemResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ItemResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\ItemResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('attempts', $data) && $data['attempts'] !== null) {
            $object->setAttempts($data['attempts']);
        }
        elseif (\array_key_exists('attempts', $data) && $data['attempts'] === null) {
            $object->setAttempts(null);
        }
        if (\array_key_exists('credential', $data) && $data['credential'] !== null) {
            $object->setCredential($data['credential']);
        }
        elseif (\array_key_exists('credential', $data) && $data['credential'] === null) {
            $object->setCredential(null);
        }
        if (\array_key_exists('lastError', $data) && $data['lastError'] !== null) {
            $object->setLastError($data['lastError']);
        }
        elseif (\array_key_exists('lastError', $data) && $data['lastError'] === null) {
            $object->setLastError(null);
        }
        if (\array_key_exists('nextAttemptAt', $data) && $data['nextAttemptAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['nextAttemptAt']);
            if (false === $date) {
                if (is_string($data['nextAttemptAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['nextAttemptAt'])) {
                    try {
                        $date = new \DateTime($data['nextAttemptAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['nextAttemptAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['nextAttemptAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setNextAttemptAt($date);
        }
        elseif (\array_key_exists('nextAttemptAt', $data) && $data['nextAttemptAt'] === null) {
            $object->setNextAttemptAt(null);
        }
        if (\array_key_exists('path', $data) && $data['path'] !== null) {
            $object->setPath($data['path']);
        }
        elseif (\array_key_exists('path', $data) && $data['path'] === null) {
            $object->setPath(null);
        }
        if (\array_key_exists('publishedAt', $data) && $data['publishedAt'] !== null) {
            $date_1 = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['publishedAt']);
            if (false === $date_1) {
                if (is_string($data['publishedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['publishedAt'])) {
                    try {
                        $date_1 = new \DateTime($data['publishedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['publishedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['publishedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setPublishedAt($date_1);
        }
        elseif (\array_key_exists('publishedAt', $data) && $data['publishedAt'] === null) {
            $object->setPublishedAt(null);
        }
        if (\array_key_exists('reference', $data) && $data['reference'] !== null) {
            $object->setReference($data['reference']);
        }
        elseif (\array_key_exists('reference', $data) && $data['reference'] === null) {
            $object->setReference(null);
        }
        if (\array_key_exists('state', $data) && $data['state'] !== null) {
            $object->setState($data['state']);
        }
        elseif (\array_key_exists('state', $data) && $data['state'] === null) {
            $object->setState(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['attempts'] = $data->getAttempts();
        $dataArray['credential'] = $data->getCredential();
        if ($data->isInitialized('lastError') && null !== $data->getLastError()) {
            $dataArray['lastError'] = $data->getLastError();
        }
        if ($data->isInitialized('nextAttemptAt') && null !== $data->getNextAttemptAt()) {
            $dataArray['nextAttemptAt'] = $data->getNextAttemptAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['path'] = $data->getPath();
        if ($data->isInitialized('publishedAt') && null !== $data->getPublishedAt()) {
            $dataArray['publishedAt'] = $data->getPublishedAt()->format('Y-m-d\TH:i:sP');
        }
        $dataArray['reference'] = $data->getReference();
        $dataArray['state'] = $data->getState();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ItemResponse::class => false];
    }
}