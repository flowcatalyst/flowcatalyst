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
class ApiSdkRolesRoleNamePutResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200();
        if (\array_key_exists('clientManaged', $data) && \is_int($data['clientManaged'])) {
            $data['clientManaged'] = (bool) $data['clientManaged'];
        }
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
            unset($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('applicationCode', $data) && $data['applicationCode'] !== null) {
            $object->setApplicationCode($data['applicationCode']);
            unset($data['applicationCode']);
        }
        elseif (\array_key_exists('applicationCode', $data) && $data['applicationCode'] === null) {
            $object->setApplicationCode(null);
        }
        if (\array_key_exists('displayName', $data) && $data['displayName'] !== null) {
            $object->setDisplayName($data['displayName']);
            unset($data['displayName']);
        }
        elseif (\array_key_exists('displayName', $data) && $data['displayName'] === null) {
            $object->setDisplayName(null);
        }
        if (\array_key_exists('shortName', $data) && $data['shortName'] !== null) {
            $object->setShortName($data['shortName']);
            unset($data['shortName']);
        }
        elseif (\array_key_exists('shortName', $data) && $data['shortName'] === null) {
            $object->setShortName(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
            unset($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('permissions', $data) && $data['permissions'] !== null) {
            $values = [];
            foreach ($data['permissions'] as $value) {
                $values[] = $value;
            }
            $object->setPermissions($values);
            unset($data['permissions']);
        }
        elseif (\array_key_exists('permissions', $data) && $data['permissions'] === null) {
            $object->setPermissions(null);
        }
        if (\array_key_exists('source', $data) && $data['source'] !== null) {
            $object->setSource($data['source']);
            unset($data['source']);
        }
        elseif (\array_key_exists('source', $data) && $data['source'] === null) {
            $object->setSource(null);
        }
        if (\array_key_exists('clientManaged', $data) && $data['clientManaged'] !== null) {
            $object->setClientManaged($data['clientManaged']);
            unset($data['clientManaged']);
        }
        elseif (\array_key_exists('clientManaged', $data) && $data['clientManaged'] === null) {
            $object->setClientManaged(null);
        }
        if (\array_key_exists('createdAt', $data) && $data['createdAt'] !== null) {
            $object->setCreatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['createdAt']));
            unset($data['createdAt']);
        }
        elseif (\array_key_exists('createdAt', $data) && $data['createdAt'] === null) {
            $object->setCreatedAt(null);
        }
        if (\array_key_exists('updatedAt', $data) && $data['updatedAt'] !== null) {
            $object->setUpdatedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['updatedAt']));
            unset($data['updatedAt']);
        }
        elseif (\array_key_exists('updatedAt', $data) && $data['updatedAt'] === null) {
            $object->setUpdatedAt(null);
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
        $dataArray['name'] = $data->getName();
        $dataArray['applicationCode'] = $data->getApplicationCode();
        $dataArray['displayName'] = $data->getDisplayName();
        $dataArray['shortName'] = $data->getShortName();
        $dataArray['description'] = $data->getDescription();
        $values = [];
        foreach ($data->getPermissions() as $value) {
            $values[] = $value;
        }
        $dataArray['permissions'] = $values;
        $dataArray['source'] = $data->getSource();
        $dataArray['clientManaged'] = $data->getClientManaged();
        $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value_1) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200::class => false];
    }
}