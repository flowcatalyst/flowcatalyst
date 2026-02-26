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
class ApiAdminServiceAccountsIdRolesPutResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('roles', $data) && $data['roles'] !== null) {
            $values = [];
            foreach ($data['roles'] as $value) {
                $values[] = $this->denormalizer->denormalize($value, \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200RolesItem::class, 'json', $context);
            }
            $object->setRoles($values);
            unset($data['roles']);
        }
        elseif (\array_key_exists('roles', $data) && $data['roles'] === null) {
            $object->setRoles(null);
        }
        if (\array_key_exists('addedRoles', $data) && $data['addedRoles'] !== null) {
            $values_1 = [];
            foreach ($data['addedRoles'] as $value_1) {
                $values_1[] = $value_1;
            }
            $object->setAddedRoles($values_1);
            unset($data['addedRoles']);
        }
        elseif (\array_key_exists('addedRoles', $data) && $data['addedRoles'] === null) {
            $object->setAddedRoles(null);
        }
        if (\array_key_exists('removedRoles', $data) && $data['removedRoles'] !== null) {
            $values_2 = [];
            foreach ($data['removedRoles'] as $value_2) {
                $values_2[] = $value_2;
            }
            $object->setRemovedRoles($values_2);
            unset($data['removedRoles']);
        }
        elseif (\array_key_exists('removedRoles', $data) && $data['removedRoles'] === null) {
            $object->setRemovedRoles(null);
        }
        foreach ($data as $key => $value_3) {
            if (preg_match('/.*/', (string) $key)) {
                $object[$key] = $value_3;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $values = [];
        foreach ($data->getRoles() as $value) {
            $values[] = $this->normalizer->normalize($value, 'json', $context);
        }
        $dataArray['roles'] = $values;
        $values_1 = [];
        foreach ($data->getAddedRoles() as $value_1) {
            $values_1[] = $value_1;
        }
        $dataArray['addedRoles'] = $values_1;
        $values_2 = [];
        foreach ($data->getRemovedRoles() as $value_2) {
            $values_2[] = $value_2;
        }
        $dataArray['removedRoles'] = $values_2;
        foreach ($data as $key => $value_3) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value_3;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200::class => false];
    }
}