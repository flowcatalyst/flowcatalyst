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
class FieldUnlockKeyAttestationNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\FieldUnlockKeyAttestation::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\FieldUnlockKeyAttestation::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $object = new \FlowCatalyst\Generated\Model\FieldUnlockKeyAttestation();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (isset($data['$ref']) && !isset($data['type']) && !isset($data['properties']) && !isset($data['allOf'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        if (\array_key_exists('authPerUse', $data) && \is_int($data['authPerUse'])) {
            $data['authPerUse'] = (bool) $data['authPerUse'];
        }
        if (\array_key_exists('biometricOnly', $data) && \is_int($data['biometricOnly'])) {
            $data['biometricOnly'] = (bool) $data['biometricOnly'];
        }
        if (\array_key_exists('challengeMatches', $data) && \is_int($data['challengeMatches'])) {
            $data['challengeMatches'] = (bool) $data['challengeMatches'];
        }
        if (\array_key_exists('meetsRequirement', $data) && \is_int($data['meetsRequirement'])) {
            $data['meetsRequirement'] = (bool) $data['meetsRequirement'];
        }
        if (\array_key_exists('signP256', $data) && \is_int($data['signP256'])) {
            $data['signP256'] = (bool) $data['signP256'];
        }
        if (\array_key_exists('attestationSecurityLevel', $data) && $data['attestationSecurityLevel'] !== null) {
            $object->setAttestationSecurityLevel($data['attestationSecurityLevel']);
        }
        elseif (\array_key_exists('attestationSecurityLevel', $data) && $data['attestationSecurityLevel'] === null) {
            $object->setAttestationSecurityLevel(null);
        }
        if (\array_key_exists('attestationVersion', $data) && $data['attestationVersion'] !== null) {
            $object->setAttestationVersion($data['attestationVersion']);
        }
        elseif (\array_key_exists('attestationVersion', $data) && $data['attestationVersion'] === null) {
            $object->setAttestationVersion(null);
        }
        if (\array_key_exists('authPerUse', $data) && $data['authPerUse'] !== null) {
            $object->setAuthPerUse($data['authPerUse']);
        }
        elseif (\array_key_exists('authPerUse', $data) && $data['authPerUse'] === null) {
            $object->setAuthPerUse(null);
        }
        if (\array_key_exists('biometricOnly', $data) && $data['biometricOnly'] !== null) {
            $object->setBiometricOnly($data['biometricOnly']);
        }
        elseif (\array_key_exists('biometricOnly', $data) && $data['biometricOnly'] === null) {
            $object->setBiometricOnly(null);
        }
        if (\array_key_exists('challengeMatches', $data) && $data['challengeMatches'] !== null) {
            $object->setChallengeMatches($data['challengeMatches']);
        }
        elseif (\array_key_exists('challengeMatches', $data) && $data['challengeMatches'] === null) {
            $object->setChallengeMatches(null);
        }
        if (\array_key_exists('checkedAt', $data) && $data['checkedAt'] !== null) {
            $date = \DateTime::createFromFormat('Y-m-d\TH:i:sP', $data['checkedAt']);
            if (false === $date) {
                if (is_string($data['checkedAt']) and 1 === preg_match('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/', $data['checkedAt'])) {
                    try {
                        $date = new \DateTime($data['checkedAt']);
                    } catch (\Exception) {
                        throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['checkedAt'], 'Y-m-d\TH:i:sP');
                    }
                } else {
                    throw new \FlowCatalyst\Generated\Runtime\Normalizer\InvalidDateException($data['checkedAt'], 'Y-m-d\TH:i:sP');
                }
            }
            $object->setCheckedAt($date);
        }
        elseif (\array_key_exists('checkedAt', $data) && $data['checkedAt'] === null) {
            $object->setCheckedAt(null);
        }
        if (\array_key_exists('dates', $data) && $data['dates'] !== null) {
            $object->setDates($data['dates']);
        }
        elseif (\array_key_exists('dates', $data) && $data['dates'] === null) {
            $object->setDates(null);
        }
        if (\array_key_exists('datesProblem', $data) && $data['datesProblem'] !== null) {
            $object->setDatesProblem($data['datesProblem']);
        }
        elseif (\array_key_exists('datesProblem', $data) && $data['datesProblem'] === null) {
            $object->setDatesProblem(null);
        }
        if (\array_key_exists('format', $data) && $data['format'] !== null) {
            $object->setFormat($data['format']);
        }
        elseif (\array_key_exists('format', $data) && $data['format'] === null) {
            $object->setFormat(null);
        }
        if (\array_key_exists('meetsRequirement', $data) && $data['meetsRequirement'] !== null) {
            $object->setMeetsRequirement($data['meetsRequirement']);
        }
        elseif (\array_key_exists('meetsRequirement', $data) && $data['meetsRequirement'] === null) {
            $object->setMeetsRequirement(null);
        }
        if (\array_key_exists('problem', $data) && $data['problem'] !== null) {
            $object->setProblem($data['problem']);
        }
        elseif (\array_key_exists('problem', $data) && $data['problem'] === null) {
            $object->setProblem(null);
        }
        if (\array_key_exists('provisioning', $data) && $data['provisioning'] !== null) {
            $object->setProvisioning($data['provisioning']);
        }
        elseif (\array_key_exists('provisioning', $data) && $data['provisioning'] === null) {
            $object->setProvisioning(null);
        }
        if (\array_key_exists('revocation', $data) && $data['revocation'] !== null) {
            $object->setRevocation($data['revocation']);
        }
        elseif (\array_key_exists('revocation', $data) && $data['revocation'] === null) {
            $object->setRevocation(null);
        }
        if (\array_key_exists('rootFingerprint', $data) && $data['rootFingerprint'] !== null) {
            $object->setRootFingerprint($data['rootFingerprint']);
        }
        elseif (\array_key_exists('rootFingerprint', $data) && $data['rootFingerprint'] === null) {
            $object->setRootFingerprint(null);
        }
        if (\array_key_exists('securityLevel', $data) && $data['securityLevel'] !== null) {
            $object->setSecurityLevel($data['securityLevel']);
        }
        elseif (\array_key_exists('securityLevel', $data) && $data['securityLevel'] === null) {
            $object->setSecurityLevel(null);
        }
        if (\array_key_exists('signP256', $data) && $data['signP256'] !== null) {
            $object->setSignP256($data['signP256']);
        }
        elseif (\array_key_exists('signP256', $data) && $data['signP256'] === null) {
            $object->setSignP256(null);
        }
        if (\array_key_exists('verdict', $data) && $data['verdict'] !== null) {
            $object->setVerdict($data['verdict']);
        }
        elseif (\array_key_exists('verdict', $data) && $data['verdict'] === null) {
            $object->setVerdict(null);
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        if ($data->isInitialized('attestationSecurityLevel') && null !== $data->getAttestationSecurityLevel()) {
            $dataArray['attestationSecurityLevel'] = $data->getAttestationSecurityLevel();
        }
        if ($data->isInitialized('attestationVersion') && null !== $data->getAttestationVersion()) {
            $dataArray['attestationVersion'] = $data->getAttestationVersion();
        }
        $dataArray['authPerUse'] = $data->getAuthPerUse();
        $dataArray['biometricOnly'] = $data->getBiometricOnly();
        $dataArray['challengeMatches'] = $data->getChallengeMatches();
        if ($data->isInitialized('checkedAt') && null !== $data->getCheckedAt()) {
            $dataArray['checkedAt'] = $data->getCheckedAt()->format('Y-m-d\TH:i:sP');
        }
        if ($data->isInitialized('dates') && null !== $data->getDates()) {
            $dataArray['dates'] = $data->getDates();
        }
        if ($data->isInitialized('datesProblem') && null !== $data->getDatesProblem()) {
            $dataArray['datesProblem'] = $data->getDatesProblem();
        }
        $dataArray['format'] = $data->getFormat();
        $dataArray['meetsRequirement'] = $data->getMeetsRequirement();
        if ($data->isInitialized('problem') && null !== $data->getProblem()) {
            $dataArray['problem'] = $data->getProblem();
        }
        if ($data->isInitialized('provisioning') && null !== $data->getProvisioning()) {
            $dataArray['provisioning'] = $data->getProvisioning();
        }
        if ($data->isInitialized('revocation') && null !== $data->getRevocation()) {
            $dataArray['revocation'] = $data->getRevocation();
        }
        if ($data->isInitialized('rootFingerprint') && null !== $data->getRootFingerprint()) {
            $dataArray['rootFingerprint'] = $data->getRootFingerprint();
        }
        if ($data->isInitialized('securityLevel') && null !== $data->getSecurityLevel()) {
            $dataArray['securityLevel'] = $data->getSecurityLevel();
        }
        $dataArray['signP256'] = $data->getSignP256();
        $dataArray['verdict'] = $data->getVerdict();
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\FieldUnlockKeyAttestation::class => false];
    }
}