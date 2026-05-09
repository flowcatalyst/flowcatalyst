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
class TokenRefreshResponseNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\TokenRefreshResponse::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\TokenRefreshResponse::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\TokenRefreshResponse();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('accessToken', $data) && $data['accessToken'] !== null) {
            $object->setAccessToken($data['accessToken']);
            unset($data['accessToken']);
        }
        elseif (\array_key_exists('accessToken', $data) && $data['accessToken'] === null) {
            $object->setAccessToken(null);
        }
        if (\array_key_exists('expiresIn', $data) && $data['expiresIn'] !== null) {
            $object->setExpiresIn($data['expiresIn']);
            unset($data['expiresIn']);
        }
        elseif (\array_key_exists('expiresIn', $data) && $data['expiresIn'] === null) {
            $object->setExpiresIn(null);
        }
        if (\array_key_exists('refreshToken', $data) && $data['refreshToken'] !== null) {
            $object->setRefreshToken($data['refreshToken']);
            unset($data['refreshToken']);
        }
        elseif (\array_key_exists('refreshToken', $data) && $data['refreshToken'] === null) {
            $object->setRefreshToken(null);
        }
        if (\array_key_exists('tokenType', $data) && $data['tokenType'] !== null) {
            $object->setTokenType($data['tokenType']);
            unset($data['tokenType']);
        }
        elseif (\array_key_exists('tokenType', $data) && $data['tokenType'] === null) {
            $object->setTokenType(null);
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
        $dataArray['accessToken'] = $data->getAccessToken();
        $dataArray['expiresIn'] = $data->getExpiresIn();
        $dataArray['refreshToken'] = $data->getRefreshToken();
        $dataArray['tokenType'] = $data->getTokenType();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\TokenRefreshResponse::class => false];
    }
}