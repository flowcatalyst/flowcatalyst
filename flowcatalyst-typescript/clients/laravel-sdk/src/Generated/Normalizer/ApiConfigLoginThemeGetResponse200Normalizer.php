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
class ApiConfigLoginThemeGetResponse200Normalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\ApiConfigLoginThemeGetResponse200::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\ApiConfigLoginThemeGetResponse200::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\ApiConfigLoginThemeGetResponse200();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('brandName', $data) && $data['brandName'] !== null) {
            $object->setBrandName($data['brandName']);
            unset($data['brandName']);
        }
        elseif (\array_key_exists('brandName', $data) && $data['brandName'] === null) {
            $object->setBrandName(null);
        }
        if (\array_key_exists('brandSubtitle', $data) && $data['brandSubtitle'] !== null) {
            $object->setBrandSubtitle($data['brandSubtitle']);
            unset($data['brandSubtitle']);
        }
        elseif (\array_key_exists('brandSubtitle', $data) && $data['brandSubtitle'] === null) {
            $object->setBrandSubtitle(null);
        }
        if (\array_key_exists('logoUrl', $data) && $data['logoUrl'] !== null) {
            $object->setLogoUrl($data['logoUrl']);
            unset($data['logoUrl']);
        }
        elseif (\array_key_exists('logoUrl', $data) && $data['logoUrl'] === null) {
            $object->setLogoUrl(null);
        }
        if (\array_key_exists('logoSvg', $data) && $data['logoSvg'] !== null) {
            $object->setLogoSvg($data['logoSvg']);
            unset($data['logoSvg']);
        }
        elseif (\array_key_exists('logoSvg', $data) && $data['logoSvg'] === null) {
            $object->setLogoSvg(null);
        }
        if (\array_key_exists('logoHeight', $data) && $data['logoHeight'] !== null) {
            $object->setLogoHeight($data['logoHeight']);
            unset($data['logoHeight']);
        }
        elseif (\array_key_exists('logoHeight', $data) && $data['logoHeight'] === null) {
            $object->setLogoHeight(null);
        }
        if (\array_key_exists('primaryColor', $data) && $data['primaryColor'] !== null) {
            $object->setPrimaryColor($data['primaryColor']);
            unset($data['primaryColor']);
        }
        elseif (\array_key_exists('primaryColor', $data) && $data['primaryColor'] === null) {
            $object->setPrimaryColor(null);
        }
        if (\array_key_exists('accentColor', $data) && $data['accentColor'] !== null) {
            $object->setAccentColor($data['accentColor']);
            unset($data['accentColor']);
        }
        elseif (\array_key_exists('accentColor', $data) && $data['accentColor'] === null) {
            $object->setAccentColor(null);
        }
        if (\array_key_exists('backgroundColor', $data) && $data['backgroundColor'] !== null) {
            $object->setBackgroundColor($data['backgroundColor']);
            unset($data['backgroundColor']);
        }
        elseif (\array_key_exists('backgroundColor', $data) && $data['backgroundColor'] === null) {
            $object->setBackgroundColor(null);
        }
        if (\array_key_exists('backgroundGradient', $data) && $data['backgroundGradient'] !== null) {
            $object->setBackgroundGradient($data['backgroundGradient']);
            unset($data['backgroundGradient']);
        }
        elseif (\array_key_exists('backgroundGradient', $data) && $data['backgroundGradient'] === null) {
            $object->setBackgroundGradient(null);
        }
        if (\array_key_exists('footerText', $data) && $data['footerText'] !== null) {
            $object->setFooterText($data['footerText']);
            unset($data['footerText']);
        }
        elseif (\array_key_exists('footerText', $data) && $data['footerText'] === null) {
            $object->setFooterText(null);
        }
        if (\array_key_exists('customCss', $data) && $data['customCss'] !== null) {
            $object->setCustomCss($data['customCss']);
            unset($data['customCss']);
        }
        elseif (\array_key_exists('customCss', $data) && $data['customCss'] === null) {
            $object->setCustomCss(null);
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
        $dataArray['brandName'] = $data->getBrandName();
        $dataArray['brandSubtitle'] = $data->getBrandSubtitle();
        $dataArray['logoUrl'] = $data->getLogoUrl();
        $dataArray['logoSvg'] = $data->getLogoSvg();
        $dataArray['logoHeight'] = $data->getLogoHeight();
        $dataArray['primaryColor'] = $data->getPrimaryColor();
        $dataArray['accentColor'] = $data->getAccentColor();
        $dataArray['backgroundColor'] = $data->getBackgroundColor();
        $dataArray['backgroundGradient'] = $data->getBackgroundGradient();
        $dataArray['footerText'] = $data->getFooterText();
        $dataArray['customCss'] = $data->getCustomCss();
        foreach ($data as $key => $value) {
            if (preg_match('/.*/', (string) $key)) {
                $dataArray[$key] = $value;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\ApiConfigLoginThemeGetResponse200::class => false];
    }
}