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
class DomainCheckResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\DomainCheckResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\DomainCheckResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\DomainCheckResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('authMethod', $data) && $data['authMethod'] !== null) {
            $object->setAuthMethod($data['authMethod']);
            unset($data['authMethod']);
        }
        elseif (\array_key_exists('authMethod', $data) && $data['authMethod'] === null) {
            $object->setAuthMethod(null);
        }
        if (\array_key_exists('loginUrl', $data) && $data['loginUrl'] !== null) {
            $object->setLoginUrl($data['loginUrl']);
            unset($data['loginUrl']);
        }
        elseif (\array_key_exists('loginUrl', $data) && $data['loginUrl'] === null) {
            $object->setLoginUrl(null);
        }
        if (\array_key_exists('idpIssuer', $data) && $data['idpIssuer'] !== null) {
            $object->setIdpIssuer($data['idpIssuer']);
            unset($data['idpIssuer']);
        }
        elseif (\array_key_exists('idpIssuer', $data) && $data['idpIssuer'] === null) {
            $object->setIdpIssuer(null);
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
        if ($data->isInitialized('authMethod') && null !== $data->getAuthMethod()) {
            $dataArray['authMethod'] = $data->getAuthMethod();
        }
        if ($data->isInitialized('loginUrl') && null !== $data->getLoginUrl()) {
            $dataArray['loginUrl'] = $data->getLoginUrl();
        }
        if ($data->isInitialized('idpIssuer') && null !== $data->getIdpIssuer()) {
            $dataArray['idpIssuer'] = $data->getIdpIssuer();
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
        return [\FlowCatalyst\Generated\Model\DomainCheckResponse::class => false];
    }
}