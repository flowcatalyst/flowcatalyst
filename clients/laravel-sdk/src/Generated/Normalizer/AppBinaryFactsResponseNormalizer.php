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
class AppBinaryFactsResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\AppBinaryFactsResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\AppBinaryFactsResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\AppBinaryFactsResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('debugCertificate', $data) && \is_int($data['debugCertificate'])) {
            $data['debugCertificate'] = (bool) $data['debugCertificate'];
        }
        if (\array_key_exists('debuggable', $data) && \is_int($data['debuggable'])) {
            $data['debuggable'] = (bool) $data['debuggable'];
        }
        if (\array_key_exists('debugCertificate', $data) && $data['debugCertificate'] !== null) {
            $object->setDebugCertificate($data['debugCertificate']);
        }
        elseif (\array_key_exists('debugCertificate', $data) && $data['debugCertificate'] === null) {
            $object->setDebugCertificate(null);
        }
        if (\array_key_exists('debuggable', $data) && $data['debuggable'] !== null) {
            $object->setDebuggable($data['debuggable']);
        }
        elseif (\array_key_exists('debuggable', $data) && $data['debuggable'] === null) {
            $object->setDebuggable(null);
        }
        if (\array_key_exists('minSdk', $data) && $data['minSdk'] !== null) {
            $object->setMinSdk($data['minSdk']);
        }
        elseif (\array_key_exists('minSdk', $data) && $data['minSdk'] === null) {
            $object->setMinSdk(null);
        }
        if (\array_key_exists('packageName', $data) && $data['packageName'] !== null) {
            $object->setPackageName($data['packageName']);
        }
        elseif (\array_key_exists('packageName', $data) && $data['packageName'] === null) {
            $object->setPackageName(null);
        }
        if (\array_key_exists('platform', $data) && $data['platform'] !== null) {
            $object->setPlatform($data['platform']);
        }
        elseif (\array_key_exists('platform', $data) && $data['platform'] === null) {
            $object->setPlatform(null);
        }
        if (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] !== null) {
            $object->setRuntimeVersion($data['runtimeVersion']);
        }
        elseif (\array_key_exists('runtimeVersion', $data) && $data['runtimeVersion'] === null) {
            $object->setRuntimeVersion(null);
        }
        if (\array_key_exists('signingCertificateSha256', $data) && $data['signingCertificateSha256'] !== null) {
            $object->setSigningCertificateSha256($data['signingCertificateSha256']);
        }
        elseif (\array_key_exists('signingCertificateSha256', $data) && $data['signingCertificateSha256'] === null) {
            $object->setSigningCertificateSha256(null);
        }
        if (\array_key_exists('signingCertificateSubject', $data) && $data['signingCertificateSubject'] !== null) {
            $object->setSigningCertificateSubject($data['signingCertificateSubject']);
        }
        elseif (\array_key_exists('signingCertificateSubject', $data) && $data['signingCertificateSubject'] === null) {
            $object->setSigningCertificateSubject(null);
        }
        if (\array_key_exists('signingSchemes', $data) && $data['signingSchemes'] !== null) {
            $values = [];
            foreach ($data['signingSchemes'] as $value) {
                $values[] = $value;
            }
            $object->setSigningSchemes($values);
        }
        elseif (\array_key_exists('signingSchemes', $data) && $data['signingSchemes'] === null) {
            $object->setSigningSchemes(null);
        }
        if (\array_key_exists('updateChannel', $data) && $data['updateChannel'] !== null) {
            $object->setUpdateChannel($data['updateChannel']);
        }
        elseif (\array_key_exists('updateChannel', $data) && $data['updateChannel'] === null) {
            $object->setUpdateChannel(null);
        }
        if (\array_key_exists('updateKeyId', $data) && $data['updateKeyId'] !== null) {
            $object->setUpdateKeyId($data['updateKeyId']);
        }
        elseif (\array_key_exists('updateKeyId', $data) && $data['updateKeyId'] === null) {
            $object->setUpdateKeyId(null);
        }
        if (\array_key_exists('updateUrl', $data) && $data['updateUrl'] !== null) {
            $object->setUpdateUrl($data['updateUrl']);
        }
        elseif (\array_key_exists('updateUrl', $data) && $data['updateUrl'] === null) {
            $object->setUpdateUrl(null);
        }
        if (\array_key_exists('versionCode', $data) && $data['versionCode'] !== null) {
            $object->setVersionCode($data['versionCode']);
        }
        elseif (\array_key_exists('versionCode', $data) && $data['versionCode'] === null) {
            $object->setVersionCode(null);
        }
        if (\array_key_exists('versionName', $data) && $data['versionName'] !== null) {
            $object->setVersionName($data['versionName']);
        }
        elseif (\array_key_exists('versionName', $data) && $data['versionName'] === null) {
            $object->setVersionName(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['debugCertificate'] = $data->getDebugCertificate();
        $dataArray['debuggable'] = $data->getDebuggable();
        $dataArray['minSdk'] = $data->getMinSdk();
        $dataArray['packageName'] = $data->getPackageName();
        $dataArray['platform'] = $data->getPlatform();
        if ($data->isInitialized('runtimeVersion') && null !== $data->getRuntimeVersion()) {
            $dataArray['runtimeVersion'] = $data->getRuntimeVersion();
        }
        $dataArray['signingCertificateSha256'] = $data->getSigningCertificateSha256();
        $dataArray['signingCertificateSubject'] = $data->getSigningCertificateSubject();
        $values = [];
        foreach ($data->getSigningSchemes() as $value) {
            $values[] = $value;
        }
        $dataArray['signingSchemes'] = $values;
        if ($data->isInitialized('updateChannel') && null !== $data->getUpdateChannel()) {
            $dataArray['updateChannel'] = $data->getUpdateChannel();
        }
        if ($data->isInitialized('updateKeyId') && null !== $data->getUpdateKeyId()) {
            $dataArray['updateKeyId'] = $data->getUpdateKeyId();
        }
        if ($data->isInitialized('updateUrl') && null !== $data->getUpdateUrl()) {
            $dataArray['updateUrl'] = $data->getUpdateUrl();
        }
        $dataArray['versionCode'] = $data->getVersionCode();
        if ($data->isInitialized('versionName') && null !== $data->getVersionName()) {
            $dataArray['versionName'] = $data->getVersionName();
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\AppBinaryFactsResponse::class => false];
    }
}