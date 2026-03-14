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
class ApiAdminServiceAccountsPostResponse201Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('serviceAccount', $data) && $data['serviceAccount'] !== null) {
            $object->setServiceAccount($this->denormalizer->denormalize($data['serviceAccount'], \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201ServiceAccount::class, 'json', $context));
            unset($data['serviceAccount']);
        }
        elseif (\array_key_exists('serviceAccount', $data) && $data['serviceAccount'] === null) {
            $object->setServiceAccount(null);
        }
        if (\array_key_exists('principalId', $data) && $data['principalId'] !== null) {
            $object->setPrincipalId($data['principalId']);
            unset($data['principalId']);
        }
        elseif (\array_key_exists('principalId', $data) && $data['principalId'] === null) {
            $object->setPrincipalId(null);
        }
        if (\array_key_exists('oauth', $data) && $data['oauth'] !== null) {
            $object->setOauth($this->denormalizer->denormalize($data['oauth'], \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201Oauth::class, 'json', $context));
            unset($data['oauth']);
        }
        elseif (\array_key_exists('oauth', $data) && $data['oauth'] === null) {
            $object->setOauth(null);
        }
        if (\array_key_exists('webhook', $data) && $data['webhook'] !== null) {
            $object->setWebhook($this->denormalizer->denormalize($data['webhook'], \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201Webhook::class, 'json', $context));
            unset($data['webhook']);
        }
        elseif (\array_key_exists('webhook', $data) && $data['webhook'] === null) {
            $object->setWebhook(null);
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
        $dataArray['serviceAccount'] = $this->normalizer->normalize($data->getServiceAccount(), 'json', $context);
        $dataArray['principalId'] = $data->getPrincipalId();
        $dataArray['oauth'] = $this->normalizer->normalize($data->getOauth(), 'json', $context);
        $dataArray['webhook'] = $this->normalizer->normalize($data->getWebhook(), 'json', $context);
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201::class => false];
    }
}