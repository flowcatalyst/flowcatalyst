import { ref, computed, watch } from "vue";
import { useRouter, useRoute } from "vue-router";
import {
	eventTypesApi,
	type EventType,
	type EventTypeFilters,
	type EventTypeStatus,
} from "@/api/event-types";

const QUERY_KEYS = {
	applications: "app",
	subdomains: "sub",
	aggregates: "agg",
	status: "status",
} as const;

function parseQueryArray(value: unknown): string[] {
	if (!value) return [];
	if (Array.isArray(value)) return value.filter((v): v is string => typeof v === "string");
	if (typeof value === "string") return value.split(",").filter(Boolean);
	return [];
}

function parseQueryString(value: unknown): string | null {
	if (typeof value === "string" && value.length > 0) return value;
	return null;
}

export function useEventTypes() {
	const router = useRouter();
	const route = useRoute();

	const eventTypes = ref<EventType[]>([]);
	const initialLoading = ref(true);
	const loading = ref(false);
	const error = ref<string | null>(null);

	// Filter state — seeded from URL query on creation
	const selectedApplications = ref<string[]>(parseQueryArray(route.query[QUERY_KEYS.applications]));
	const selectedSubdomains = ref<string[]>(parseQueryArray(route.query[QUERY_KEYS.subdomains]));
	const selectedAggregates = ref<string[]>(parseQueryArray(route.query[QUERY_KEYS.aggregates]));
	const selectedStatus = ref<string | null>(parseQueryString(route.query[QUERY_KEYS.status]));

	// Filter options
	const applicationOptions = ref<string[]>([]);
	const subdomainOptions = ref<string[]>([]);
	const aggregateOptions = ref<string[]>([]);

	const statusOptions = [
		{ label: "Current", value: "CURRENT" },
		{ label: "Archived", value: "ARCHIVED" },
	];

	const hasActiveFilters = computed(() => {
		return (
			selectedApplications.value.length > 0 ||
			selectedSubdomains.value.length > 0 ||
			selectedAggregates.value.length > 0 ||
			selectedStatus.value !== null
		);
	});

	// ---- URL sync ----

	let suppressUrlSync = false;

	function syncFiltersToUrl() {
		if (suppressUrlSync) return;

		const query: Record<string, string | undefined> = {};
		if (selectedApplications.value.length > 0)
			query[QUERY_KEYS.applications] = selectedApplications.value.join(",");
		if (selectedSubdomains.value.length > 0)
			query[QUERY_KEYS.subdomains] = selectedSubdomains.value.join(",");
		if (selectedAggregates.value.length > 0)
			query[QUERY_KEYS.aggregates] = selectedAggregates.value.join(",");
		if (selectedStatus.value)
			query[QUERY_KEYS.status] = selectedStatus.value;

		router.replace({ query });
	}

	// ---- Data loading ----

	async function loadEventTypes() {
		loading.value = true;
		error.value = null;

		try {
			const filters: EventTypeFilters = {};
			if (selectedApplications.value.length)
				filters.applications = selectedApplications.value;
			if (selectedSubdomains.value.length)
				filters.subdomains = selectedSubdomains.value;
			if (selectedAggregates.value.length)
				filters.aggregates = selectedAggregates.value;
			if (selectedStatus.value) filters.status = selectedStatus.value as EventTypeStatus;

			const response = await eventTypesApi.list(filters);
			eventTypes.value = response.items;
		} catch (e) {
			error.value =
				e instanceof Error ? e.message : "Failed to load event types";
		} finally {
			loading.value = false;
		}
	}

	async function loadApplications() {
		const response = await eventTypesApi.getApplications();
		applicationOptions.value = response.options;

		// Prune selections that no longer exist
		const valid = new Set(response.options);
		const pruned = selectedApplications.value.filter((s) => valid.has(s));
		if (pruned.length !== selectedApplications.value.length) {
			suppressUrlSync = true;
			selectedApplications.value = pruned;
			suppressUrlSync = false;
		}
	}

	async function loadSubdomains() {
		const apps = selectedApplications.value.length
			? selectedApplications.value
			: undefined;
		const response = await eventTypesApi.getSubdomains(apps);
		subdomainOptions.value = response.options;

		// Prune invalid selections
		const pruned = selectedSubdomains.value.filter((s) =>
			response.options.includes(s),
		);
		if (pruned.length !== selectedSubdomains.value.length) {
			suppressUrlSync = true;
			selectedSubdomains.value = pruned;
			suppressUrlSync = false;
		}
	}

	async function loadAggregates() {
		const apps = selectedApplications.value.length
			? selectedApplications.value
			: undefined;
		const subs = selectedSubdomains.value.length
			? selectedSubdomains.value
			: undefined;
		const response = await eventTypesApi.getAggregates(apps, subs);
		aggregateOptions.value = response.options;

		// Prune invalid selections
		const pruned = selectedAggregates.value.filter((a) =>
			response.options.includes(a),
		);
		if (pruned.length !== selectedAggregates.value.length) {
			suppressUrlSync = true;
			selectedAggregates.value = pruned;
			suppressUrlSync = false;
		}
	}

	function clearFilters() {
		selectedApplications.value = [];
		selectedSubdomains.value = [];
		selectedAggregates.value = [];
		selectedStatus.value = null;
	}

	// Watch for filter changes — sync URL + reload data
	watch(selectedApplications, () => {
		syncFiltersToUrl();
		loadSubdomains();
		loadAggregates();
		loadEventTypes();
	});

	watch(selectedSubdomains, () => {
		syncFiltersToUrl();
		loadAggregates();
		loadEventTypes();
	});

	watch(selectedAggregates, () => {
		syncFiltersToUrl();
		loadEventTypes();
	});

	watch(selectedStatus, () => {
		syncFiltersToUrl();
		loadEventTypes();
	});

	async function initialize() {
		await Promise.all([loadApplications(), loadSubdomains(), loadAggregates()]);
		// After pruning, sync the (possibly cleaned) state back to URL
		syncFiltersToUrl();
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
