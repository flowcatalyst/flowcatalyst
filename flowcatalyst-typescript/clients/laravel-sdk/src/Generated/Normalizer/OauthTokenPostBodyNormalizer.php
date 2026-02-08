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
class OauthTokenPostBodyNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\OauthTokenPostBody::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\OauthTokenPostBody::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\OauthTokenPostBody();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('grant_type', $data) && $data['grant_type'] !== null) {
            $object->setGrantType($data['grant_type']);
            unset($data['grant_type']);
        }
        elseif (\array_key_exists('grant_type', $data) && $data['grant_type'] === null) {
            $object->setGrantType(null);
        }
        if (\array_key_exists('code', $data) && $data['code'] !== null) {
            $object->setCode($data['code']);
            unset($data['code']);
        }
        elseif (\array_key_exists('code', $data) && $data['code'] === null) {
            $object->setCode(null);
        }
        if (\array_key_exists('redirect_uri', $data) && $data['redirect_uri'] !== null) {
            $object->setRedirectUri($data['redirect_uri']);
            unset($data['redirect_uri']);
        }
        elseif (\array_key_exists('redirect_uri', $data) && $data['redirect_uri'] === null) {
            $object->setRedirectUri(null);
        }
        if (\array_key_exists('client_id', $data) && $data['client_id'] !== null) {
            $object->setClientId($data['client_id']);
            unset($data['client_id']);
        }
        elseif (\array_key_exists('client_id', $data) && $data['client_id'] === null) {
            $object->setClientId(null);
        }
        if (\array_key_exists('client_secret', $data) && $data['client_secret'] !== null) {
            $object->setClientSecret($data['client_secret']);
            unset($data['client_secret']);
        }
        elseif (\array_key_exists('client_secret', $data) && $data['client_secret'] === null) {
            $object->setClientSecret(null);
        }
        if (\array_key_exists('code_verifier', $data) && $data['code_verifier'] !== null) {
            $object->setCodeVerifier($data['code_verifier']);
            unset($data['code_verifier']);
        }
        elseif (\array_key_exists('code_verifier', $data) && $data['code_verifier'] === null) {
            $object->setCodeVerifier(null);
        }
        if (\array_key_exists('refresh_token', $data) && $data['refresh_token'] !== null) {
            $object->setRefreshToken($data['refresh_token']);
            unset($data['refresh_token']);
        }
        elseif (\array_key_exists('refresh_token', $data) && $data['refresh_token'] === null) {
            $object->setRefreshToken(null);
        }
        if (\array_key_exists('username', $data) && $data['username'] !== null) {
            $object->setUsername($data['username']);
            unset($data['username']);
        }
        elseif (\array_key_exists('username', $data) && $data['username'] === null) {
            $object->setUsername(null);
        }
        if (\array_key_exists('password', $data) && $data['password'] !== null) {
            $object->setPassword($data['password']);
            unset($data['password']);
        }
        elseif (\array_key_exists('password', $data) && $data['password'] === null) {
            $object->setPassword(null);
        }
        if (\array_key_exists('scope', $data) && $data['scope'] !== null) {
            $object->setScope($data['scope']);
            unset($data['scope']);
        }
        elseif (\array_key_exists('scope', $data) && $data['scope'] === null) {
            $object->setScope(null);
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
        if ($data->isInitialized('grantType') && null !== $data->getGrantType()) {
            $dataArray['grant_type'] = $data->getGrantType();
        }
        if ($data->isInitialized('code') && null !== $data->getCode()) {
            $dataArray['code'] = $data->getCode();
        }
        if ($data->isInitialized('redirectUri') && null !== $data->getRedirectUri()) {
            $dataArray['redirect_uri'] = $data->getRedirectUri();
        }
        if ($data->isInitialized('clientId') && null !== $data->getClientId()) {
            $dataArray['client_id'] = $data->getClientId();
        }
        if ($data->isInitialized('clientSecret') && null !== $data->getClientSecret()) {
            $dataArray['client_secret'] = $data->getClientSecret();
        }
        if ($data->isInitialized('codeVerifier') && null !== $data->getCodeVerifier()) {
            $dataArray['code_verifier'] = $data->getCodeVerifier();
        }
        if ($data->isInitialized('refreshToken') && null !== $data->getRefreshToken()) {
            $dataArray['refresh_token'] = $data->getRefreshToken();
        }
        if ($data->isInitialized('username') && null !== $data->getUsername()) {
            $dataArray['username'] = $data->getUsername();
        }
        if ($data->isInitialized('password') && null !== $data->getPassword()) {
            $dataArray['password'] = $data->getPassword();
        }
        if ($data->isInitialized('scope') && null !== $data->getScope()) {
            $dataArray['scope'] = $data->getScope();
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
        return [\FlowCatalyst\Generated\Model\OauthTokenPostBody::class => false];
    }
}