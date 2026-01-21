import { ref, computed, watch } from 'vue';
import {
  eventTypesApi,
  type EventType,
  type EventTypeFilters,
} from '@/api/event-types';

export function useEventTypes() {
  const eventTypes = ref<EventType[]>([]);
  const initialLoading = ref(true);  // For first load - shows spinner
  const loading = ref(false);         // For subsequent loads - uses DataTable loading
  const error = ref<string | null>(null);

  // Filter state
  const selectedApplications = ref<string[]>([]);
  const selectedSubdomains = ref<string[]>([]);
  const selectedAggregates = ref<string[]>([]);
  const selectedStatus = ref<string | null>(null);

  // Filter options
  const applicationOptions = ref<string[]>([]);
  const subdomainOptions = ref<string[]>([]);
  const aggregateOptions = ref<string[]>([]);

  const statusOptions = [
    { label: 'Current', value: 'CURRENT' },
    { label: 'Archived', value: 'ARCHIVE' },
  ];

  const hasActiveFilters = computed(() => {
    return (
      selectedApplications.value.length > 0 ||
      selectedSubdomains.value.length > 0 ||
      selectedAggregates.value.length > 0 ||
      selectedStatus.value !== null
    );
  });

  async function loadEventTypes() {
    loading.value = true;
    error.value = null;

    try {
      const filters: EventTypeFilters = {};
      if (selectedApplications.value.length) filters.applications = selectedApplications.value;
      if (selectedSubdomains.value.length) filters.subdomains = selectedSubdomains.value;
      if (selectedAggregates.value.length) filters.aggregates = selectedAggregates.value;
      if (selectedStatus.value) filters.status = selectedStatus.value as any;

      const response = await eventTypesApi.list(filters);
      eventTypes.value = response.items;
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load event types';
    } finally {
      loading.value = false;
    }
  }

  async function loadApplications() {
    const response = await eventTypesApi.getApplications();
    applicationOptions.value = response.options;
  }

  async function loadSubdomains() {
    const apps = selectedApplications.value.length ? selectedApplications.value : undefined;
    const response = await eventTypesApi.getSubdomains(apps);
    subdomainOptions.value = response.options;

    // Filter out invalid selections
    selectedSubdomains.value = selectedSubdomains.value.filter(s =>
      response.options.includes(s)
    );
  }

  async function loadAggregates() {
    const apps = selectedApplications.value.length ? selectedApplications.value : undefined;
    const subs = selectedSubdomains.value.length ? selectedSubdomains.value : undefined;
    const response = await eventTypesApi.getAggregates(apps, subs);
    aggregateOptions.value = response.options;

    // Filter out invalid selections
    selectedAggregates.value = selectedAggregates.value.filter(a =>
      response.options.includes(a)
    );
  }

  function clearFilters() {
    selectedApplications.value = [];
    selectedSubdomains.value = [];
    selectedAggregates.value = [];
    selectedStatus.value = null;
  }

  // Watch for filter changes
  watch(selectedApplications, () => {
    loadSubdomains();
    loadAggregates();
    loadEventTypes();
  });

  watch(selectedSubdomains, () => {
    loadAggregates();
    loadEventTypes();
  });

  watch([selectedAggregates, selectedStatus], () => {
    loadEventTypes();
  });

  async function initialize() {
    await Promise.all([loadApplications(), loadSubdomains(), loadAggregates()]);
    await loadEventTypes();
    initialLoading.value = false;
  }

  return {
    // State
    eventTypes,
    initialLoading,
    loading,
    error,

    // Filters
    selectedApplications,
    selectedSubdomains,
    selectedAggregates,
    selectedStatus,
    hasActiveFilters,

    // Options
    applicationOptions,
    subdomainOptions,
    aggregateOptions,
    statusOptions,

    // Actions
    loadEventTypes,
    clearFilters,
    initialize,
  };
}
