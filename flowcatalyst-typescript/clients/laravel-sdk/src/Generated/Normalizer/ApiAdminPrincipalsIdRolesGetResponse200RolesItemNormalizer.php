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
class ApiAdminPrincipalsIdRolesGetResponse200RolesItemNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200RolesItem::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200RolesItem::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200RolesItem();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('roleName', $data) && $data['roleName'] !== null) {
            $object->setRoleName($data['roleName']);
            unset($data['roleName']);
        }
        elseif (\array_key_exists('roleName', $data) && $data['roleName'] === null) {
            $object->setRoleName(null);
        }
        if (\array_key_exists('assignmentSource', $data) && $data['assignmentSource'] !== null) {
            $object->setAssignmentSource($data['assignmentSource']);
            unset($data['assignmentSource']);
        }
        elseif (\array_key_exists('assignmentSource', $data) && $data['assignmentSource'] === null) {
            $object->setAssignmentSource(null);
        }
        if (\array_key_exists('assignedAt', $data) && $data['assignedAt'] !== null) {
            $object->setAssignedAt(\DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['assignedAt']));
            unset($data['assignedAt']);
        }
        elseif (\array_key_exists('assignedAt', $data) && $data['assignedAt'] === null) {
            $object->setAssignedAt(null);
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
        $dataArray['roleName'] = $data->getRoleName();
        $dataArray['assignmentSource'] = $data->getAssignmentSource();
        $dataArray['assignedAt'] = $data->getAssignedAt()->format('Y-m-d\TH:i:sP');
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200RolesItem::class => false];
    }
}