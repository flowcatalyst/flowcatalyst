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
class AuditLogListResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AuditLogListResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AuditLogListResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\AuditLogListResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('auditLogs', $data) && $data['auditLogs'] !== null) {
            $values = [];
            foreach ($data['auditLogs'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\AuditLogDto::class, 'json', $context);
            }
            $object->setAuditLogs($values);
            unset($data['auditLogs']);
        }
        elseif (\array_key_exists('auditLogs', $data) && $data['auditLogs'] === null) {
            $object->setAuditLogs(null);
        }
        if (\array_key_exists('total', $data) && $data['total'] !== null) {
            $object->setTotal($data['total']);
            unset($data['total']);
        }
        elseif (\array_key_exists('total', $data) && $data['total'] === null) {
            $object->setTotal(null);
        }
        if (\array_key_exists('page', $data) && $data['page'] !== null) {
            $object->setPage($data['page']);
            unset($data['page']);
        }
        elseif (\array_key_exists('page', $data) && $data['page'] === null) {
            $object->setPage(null);
        }
        if (\array_key_exists('pageSize', $data) && $data['pageSize'] !== null) {
            $object->setPageSize($data['pageSize']);
            unset($data['pageSize']);
        }
        elseif (\array_key_exists('pageSize', $data) && $data['pageSize'] === null) {
            $object->setPageSize(null);
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
        if ($data->isInitialized('auditLogs') && null !== $data->getAuditLogs()) {
            $values = [];
            foreach ($data->getAuditLogs() as $value) {
                $values[] = $this->normalizer->normalize($value, 'json', $context);
            }
            $dataArray['auditLogs'] = $values;
        }
        if ($data->isInitialized('total') && null !== $data->getTotal()) {
            $dataArray['total'] = $data->getTotal();
        }
        if ($data->isInitialized('page') && null !== $data->getPage()) {
            $dataArray['page'] = $data->getPage();
        }
        if ($data->isInitialized('pageSize') && null !== $data->getPageSize()) {
            $dataArray['pageSize'] = $data->getPageSize();
        }
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AuditLogListResponse::class => false];
    }
}