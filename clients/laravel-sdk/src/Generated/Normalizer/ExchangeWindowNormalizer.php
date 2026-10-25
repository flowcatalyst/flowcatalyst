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
class ExchangeWindowNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ExchangeWindow::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ExchangeWindow::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\ExchangeWindow();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('completed', $data) && $data['completed'] !== null) {
            $object->setCompleted($data['completed']);
            unset($data['completed']);
        }
        elseif (\array_key_exists('completed', $data) && $data['completed'] === null) {
            $object->setCompleted(null);
        }
        if (\array_key_exists('dispatchJobs', $data) && $data['dispatchJobs'] !== null) {
            $object->setDispatchJobs($data['dispatchJobs']);
            unset($data['dispatchJobs']);
        }
        elseif (\array_key_exists('dispatchJobs', $data) && $data['dispatchJobs'] === null) {
            $object->setDispatchJobs(null);
        }
        if (\array_key_exists('failed', $data) && $data['failed'] !== null) {
            $object->setFailed($data['failed']);
            unset($data['failed']);
        }
        elseif (\array_key_exists('failed', $data) && $data['failed'] === null) {
            $object->setFailed(null);
        }
        if (\array_key_exists('from', $data) && $data['from'] !== null) {
            $object->setFrom(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['from']));
            unset($data['from']);
        }
        elseif (\array_key_exists('from', $data) && $data['from'] === null) {
            $object->setFrom(null);
        }
        if (\array_key_exists('ingestionId', $data) && $data['ingestionId'] !== null) {
            $object->setIngestionId($data['ingestionId']);
            unset($data['ingestionId']);
        }
        elseif (\array_key_exists('ingestionId', $data) && $data['ingestionId'] === null) {
            $object->setIngestionId(null);
        }
        if (\array_key_exists('records', $data) && $data['records'] !== null) {
            $object->setRecords($data['records']);
            unset($data['records']);
        }
        elseif (\array_key_exists('records', $data) && $data['records'] === null) {
            $object->setRecords(null);
        }
        if (\array_key_exists('to', $data) && $data['to'] !== null) {
            $object->setTo(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['to']));
            unset($data['to']);
        }
        elseif (\array_key_exists('to', $data) && $data['to'] === null) {
            $object->setTo(null);
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
        $dataArray['completed'] = $data->getCompleted();
        $dataArray['dispatchJobs'] = $data->getDispatchJobs();
        $dataArray['failed'] = $data->getFailed();
        $dataArray['from'] = $data->getFrom()->format('Y-m-d\TH:i:sP');
        $dataArray['ingestionId'] = $data->getIngestionId();
        $dataArray['records'] = $data->getRecords();
        $dataArray['to'] = $data->getTo()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ExchangeWindow::class => false];
    }
}