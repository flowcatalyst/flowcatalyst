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
class ApiAdminConfigAccessAppCodeRoleCodePutResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200();
        if (\array_key_exists('canRead', $data) && \is_int($data['canRead'])) {
            $data['canRead'] = (bool) $data['canRead'];
        }
        if (\array_key_exists('canWrite', $data) && \is_int($data['canWrite'])) {
            $data['canWrite'] = (bool) $data['canWrite'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('id', $data) && $data['id'] !== null) {
            $object->setId($data['id']);
            unset($data['id']);
        }
        elseif (\array_key_exists('id', $data) && $data['id'] === null) {
            $object->setId(null);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
            unset($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('roleCode', $data) && $data['roleCode'] !== null) {
            $object->setRoleCode($data['roleCode']);
            unset($data['roleCode']);
        }
        elseif (\array_key_exists('roleCode', $data) && $data['roleCode'] === null) {
            $object->setRoleCode(null);
        }
        if (\array_key_exists('canRead', $data) && $data['canRead'] !== null) {
            $object->setCanRead($data['canRead']);
            unset($data['canRead']);
        }
        elseif (\array_key_exists('canRead', $data) && $data['canRead'] === null) {
            $object->setCanRead(null);
        }
        if (\array_key_exists('canWrite', $data) && $data['canWrite'] !== null) {
            $object->setCanWrite($data['canWrite']);
            unset($data['canWrite']);
        }
        elseif (\array_key_exists('canWrite', $data) && $data['canWrite'] === null) {
            $object->setCanWrite(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
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
        $dataArray['id'] = $data->getId();
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['roleCode'] = $data->getRoleCode();
        $dataArray['canRead'] = $data->getCanRead();
        $dataArray['canWrite'] = $data->getCanWrite();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200::class => false];
    }
}