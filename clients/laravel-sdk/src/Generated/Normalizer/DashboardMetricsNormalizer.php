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
class DashboardMetricsNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return $type === \FlowCatalyst\Generated\Model\DashboardMetrics::class;
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && get_class($data) === \FlowCatalyst\Generated\Model\DashboardMetrics::class;
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        if (isset($data['$ref'])) {
            return new Reference($data['$ref'], $context['document-origin']);
        }
        if (isset($data['$recursiveRef'])) {
            return new Reference($data['$recursiveRef'], $context['document-origin']);
        }
        $object = new \FlowCatalyst\Generated\Model\DashboardMetrics();
        if (null === $data || false === \is_array($data)) {
            return $object;
        }
        if (\array_key_exists('activePools', $data) && $data['activePools'] !== null) {
            $object->setActivePools($data['activePools']);
            unset($data['activePools']);
        }
        elseif (\array_key_exists('activePools', $data) && $data['activePools'] === null) {
            $object->setActivePools(null);
        }
        if (\array_key_exists('activeSubscriptions', $data) && $data['activeSubscriptions'] !== null) {
            $object->setActiveSubscriptions($data['activeSubscriptions']);
            unset($data['activeSubscriptions']);
        }
        elseif (\array_key_exists('activeSubscriptions', $data) && $data['activeSubscriptions'] === null) {
            $object->setActiveSubscriptions(null);
        }
        if (\array_key_exists('eventsLastHour', $data) && $data['eventsLastHour'] !== null) {
            $object->setEventsLastHour($data['eventsLastHour']);
            unset($data['eventsLastHour']);
        }
        elseif (\array_key_exists('eventsLastHour', $data) && $data['eventsLastHour'] === null) {
            $object->setEventsLastHour(null);
        }
        if (\array_key_exists('health', $data) && $data['health'] !== null) {
            $object->setHealth($this->denormalizer->denormalize($data['health'], \FlowCatalyst\Generated\Model\SystemHealth::class, 'json', $context));
            unset($data['health']);
        }
        elseif (\array_key_exists('health', $data) && $data['health'] === null) {
            $object->setHealth(null);
        }
        if (\array_key_exists('jobsByStatus', $data) && $data['jobsByStatus'] !== null) {
            $values = new \ArrayObject([], \ArrayObject::ARRAY_AS_PROPS);
            foreach ($data['jobsByStatus'] as $key => $value) {
                $values[$key] = $value;
            }
            $object->setJobsByStatus($values);
            unset($data['jobsByStatus']);
        }
        elseif (\array_key_exists('jobsByStatus', $data) && $data['jobsByStatus'] === null) {
            $object->setJobsByStatus(null);
        }
        if (\array_key_exists('totalEvents', $data) && $data['totalEvents'] !== null) {
            $object->setTotalEvents($data['totalEvents']);
            unset($data['totalEvents']);
        }
        elseif (\array_key_exists('totalEvents', $data) && $data['totalEvents'] === null) {
            $object->setTotalEvents(null);
        }
        if (\array_key_exists('totalJobs', $data) && $data['totalJobs'] !== null) {
            $object->setTotalJobs($data['totalJobs']);
            unset($data['totalJobs']);
        }
        elseif (\array_key_exists('totalJobs', $data) && $data['totalJobs'] === null) {
            $object->setTotalJobs(null);
        }
        foreach ($data as $key_1 => $value_1) {
            if (preg_match('/.*/', (string) $key_1)) {
                $object[$key_1] = $value_1;
            }
        }
        return $object;
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $dataArray = [];
        $dataArray['activePools'] = $data->getActivePools();
        $dataArray['activeSubscriptions'] = $data->getActiveSubscriptions();
        $dataArray['eventsLastHour'] = $data->getEventsLastHour();
        $dataArray['health'] = $this->normalizer->normalize($data->getHealth(), 'json', $context);
        $values = [];
        foreach ($data->getJobsByStatus() as $key => $value) {
            $values[$key] = $value;
        }
        $dataArray['jobsByStatus'] = $values;
        $dataArray['totalEvents'] = $data->getTotalEvents();
        $dataArray['totalJobs'] = $data->getTotalJobs();
        foreach ($data as $key_1 => $value_1) {
            if (preg_match('/.*/', (string) $key_1)) {
                $dataArray[$key_1] = $value_1;
            }
        }
        return $dataArray;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [\FlowCatalyst\Generated\Model\DashboardMetrics::class => false];
    }
}