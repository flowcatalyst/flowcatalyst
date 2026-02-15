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
class ApplicationResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApplicationResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApplicationResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApplicationResponse();
        if (\array_key_exists('active', $data) && \is_int($data['active'])) {
            $data['active'] = (bool) $data['active'];
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
        if (\array_key_exists('type', $data) && $data['type'] !== null) {
            $object->setType($data['type']);
            unset($data['type']);
        }
        elseif (\array_key_exists('type', $data) && $data['type'] === null) {
            $object->setType(null);
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
            unset($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('name', $data) && $data['name'] !== null) {
            $object->setName($data['name']);
            unset($data['name']);
        }
        elseif (\array_key_exists('name', $data) && $data['name'] === null) {
            $object->setName(null);
        }
        if (\array_key_exists('description', $data) && $data['description'] !== null) {
            $object->setDescription($data['description']);
            unset($data['description']);
        }
        elseif (\array_key_exists('description', $data) && $data['description'] === null) {
            $object->setDescription(null);
        }
        if (\array_key_exists('defaultBaseUrl', $data) && $data['defaultBaseUrl'] !== null) {
            $object->setDefaultBaseUrl($data['defaultBaseUrl']);
            unset($data['defaultBaseUrl']);
        }
        elseif (\array_key_exists('defaultBaseUrl', $data) && $data['defaultBaseUrl'] === null) {
            $object->setDefaultBaseUrl(null);
        }
        if (\array_key_exists('iconUrl', $data) && $data['iconUrl'] !== null) {
            $object->setIconUrl($data['iconUrl']);
            unset($data['iconUrl']);
        }
        elseif (\array_key_exists('iconUrl', $data) && $data['iconUrl'] === null) {
            $object->setIconUrl(null);
        }
        if (\array_key_exists('website', $data) && $data['website'] !== null) {
            $object->setWebsite($data['website']);
            unset($data['website']);
        }
        elseif (\array_key_exists('website', $data) && $data['website'] === null) {
            $object->setWebsite(null);
        }
        if (\array_key_exists('logo', $data) && $data['logo'] !== null) {
            $object->setLogo($data['logo']);
            unset($data['logo']);
        }
        elseif (\array_key_exists('logo', $data) && $data['logo'] === null) {
            $object->setLogo(null);
        }
        if (\array_key_exists('logoMimeType', $data) && $data['logoMimeType'] !== null) {
            $object->setLogoMimeType($data['logoMimeType']);
            unset($data['logoMimeType']);
        }
        elseif (\array_key_exists('logoMimeType', $data) && $data['logoMimeType'] === null) {
            $object->setLogoMimeType(null);
        }
        if (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] !== null) {
            $object->setServiceAccountId($data['serviceAccountId']);
            unset($data['serviceAccountId']);
        }
        elseif (\array_key_exists('serviceAccountId', $data) && $data['serviceAccountId'] === null) {
            $object->setServiceAccountId(null);
        }
        if (\array_key_exists('serviceAccountPrincipalId', $data) && $data['serviceAccountPrincipalId'] !== null) {
            $object->setServiceAccountPrincipalId($data['serviceAccountPrincipalId']);
            unset($data['serviceAccountPrincipalId']);
        }
        elseif (\array_key_exists('serviceAccountPrincipalId', $data) && $data['serviceAccountPrincipalId'] === null) {
            $object->setServiceAccountPrincipalId(null);
        }
        if (\array_key_exists('active', $data) && $data['active'] !== null) {
            $object->setActive($data['active']);
            unset($data['active']);
        }
        elseif (\array_key_exists('active', $data) && $data['active'] === null) {
            $object->setActive(null);
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
        if (\array_key_exists('serviceAccount', $data) && $data['serviceAccount'] !== null) {
            $object->setServiceAccount($this->denormalizer->denormalize($data['serviceAccount'], \FlowCatalyst\Generated\Model\ServiceAccountInfo::class, 'json', $context));
            unset($data['serviceAccount']);
        }
        elseif (\array_key_exists('serviceAccount', $data) && $data['serviceAccount'] === null) {
            $object->setServiceAccount(null);
        }
        if (\array_key_exists('warning', $data) && $data['warning'] !== null) {
            $object->setWarning($data['warning']);
            unset($data['warning']);
        }
        elseif (\array_key_exists('warning', $data) && $data['warning'] === null) {
            $object->setWarning(null);
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
        if ($data->isInitialized('id') && null !== $data->getId()) {
            $dataArray['id'] = $data->getId();
        }
        if ($data->isInitialized('type') && null !== $data->getType()) {
            $dataArray['type'] = $data->getType();
        }
        if ($data->isInitialized('code') && null !== $data->getCode()) {
            $dataArray['code'] = $data->getCode();
        }
        if ($data->isInitialized('name') && null !== $data->getName()) {
            $dataArray['name'] = $data->getName();
        }
        if ($data->isInitialized('description') && null !== $data->getDescription()) {
            $dataArray['description'] = $data->getDescription();
        }
        if ($data->isInitialized('defaultBaseUrl') && null !== $data->getDefaultBaseUrl()) {
            $dataArray['defaultBaseUrl'] = $data->getDefaultBaseUrl();
        }
        if ($data->isInitialized('iconUrl') && null !== $data->getIconUrl()) {
            $dataArray['iconUrl'] = $data->getIconUrl();
        }
        if ($data->isInitialized('website') && null !== $data->getWebsite()) {
            $dataArray['website'] = $data->getWebsite();
        }
        if ($data->isInitialized('logo') && null !== $data->getLogo()) {
            $dataArray['logo'] = $data->getLogo();
        }
        if ($data->isInitialized('logoMimeType') && null !== $data->getLogoMimeType()) {
            $dataArray['logoMimeType'] = $data->getLogoMimeType();
        }
        if ($data->isInitialized('serviceAccountId') && null !== $data->getServiceAccountId()) {
            $dataArray['serviceAccountId'] = $data->getServiceAccountId();
        }
        if ($data->isInitialized('serviceAccountPrincipalId') && null !== $data->getServiceAccountPrincipalId()) {
            $dataArray['serviceAccountPrincipalId'] = $data->getServiceAccountPrincipalId();
        }
        if ($data->isInitialized('active') && null !== $data->getActive()) {
            $dataArray['active'] = $data->getActive();
        }
        if ($data->isInitialized('createdAt') && null !== $data->getCreatedAt()) {
            $dataArray['createdAt'] = $data->getCreatedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('updatedAt') && null !== $data->getUpdatedAt()) {
            $dataArray['updatedAt'] = $data->getUpdatedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('serviceAccount') && null !== $data->getServiceAccount()) {
            $dataArray['serviceAccount'] = $this->normalizer->normalize($data->getServiceAccount(), 'json', $context);
        }
        if ($data->isInitialized('warning') && null !== $data->getWarning()) {
            $dataArray['warning'] = $data->getWarning();
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
        return [\FlowCatalyst\Generated\Model\ApplicationResponse::class => false];
    }
}