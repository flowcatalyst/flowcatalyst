<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useListState } from "@/composables/useListState";
import { useTableFilters } from "@/composables/useTableFilters";
import { useClientOptions } from "@/composables/useClientOptions";
import ClientFilter from "@/components/ClientFilter.vue";
import {
	dispatchJobsApi,
	filterBody,
	type DispatchJobFilterBody,
	type DispatchJobRead as DispatchJob,
	type DispatchJobsListParams,
	type RequeueByFilterPreviewResponse,
	type RequeueByFilterResponse,
	type RequeueResponse,
	type WindowResponse,
} from "@/api/dispatch-jobs";
import { notify, toast } from "@/utils/errorBus";
import { useAuthStore } from "@/stores/auth";
import { userHasPermission } from "@/stores/permissions";
import { DISPATCH_JOB_REDRIVE_COMPLETED, DISPATCH_JOB_REQUEUE, jobActions } from "@/utils/dispatch-job-permissions";
import RequeueByFilterDialog from "./RequeueByFilterDialog.vue";
import RequeueSelectedDialog from "./RequeueSelectedDialog.vue";
import {
	activeLens,
	explainListError,
	filtersToParams,
	lensFilters,
	MAX_EVENT_TYPES,
	suggestEventTypes,
	validateFilters,
	type JobListFilters,
	type Lens,
} from "./jobFilters";
import {
	customRangeTooLarge,
	DEFAULT_WINDOW_KEY,
	describeWindow,
	normalizePreset,
	selectPreset,
	WINDOW_PRESETS,
} from "./jobWindow";
import { codeLevels } from "@/utils/code-facets";
import {
	ERROR_STATUSES,
	requeueIdsOutcome,
	requeueOutcome,
	selectAllBannerState,
	statusSeverity,
	summarizeFilters,
	type FilterSummaryItem,
} from "./requeueByFilter";

interface FilterOption {
	label: string;
	value: string;
}

const sizeOptions = [50, 100, 200, 500, 1000];
// Requeue has its own permission (the API refuses without it): the requeue
// actions are offered only to holders.
const authStore = useAuthStore();
const canRequeue = computed(() => userHasPermission(authStore.user, DISPATCH_JOB_REQUEUE));
const canRedrive = computed(() => userHasPermission(authStore.user, DISPATCH_JOB_REDRIVE_COMPLETED));
const route = useRoute();
const router = useRouter();

// MANUAL loading: the cascading handlers below clear child refs under
// withSuppressed and call load() themselves. Do NOT pass an onChange
// callback here — its async watcher flush would escape withSuppressed and
// fire one load per cleared child ref.
const listState = useListState({
	filters: {
		clients: { type: "array", key: "clients" },
		applications: { type: "array", key: "applications" },
		subdomains: { type: "array", key: "subdomains" },
		aggregates: { type: "array", key: "aggregates" },
		codes: { type: "array", key: "codes" },
		statuses: { type: "array", key: "statuses" },
		search: { type: "string", key: "q" },
		// Exact message group — follow one aggregate's jobs in order.
		messageGroup: { type: "string", key: "group" },
		// Created-at range, URL-synced as YYYY-MM-DD.
		from: { type: "string", key: "from" },
		to: { type: "string", key: "to" },
		// Window preset ("1h" | "3d" | "7d" | "31d"); empty = the server's default (24h).
		window: { type: "string", key: "window" },
		// A job's code is its event type; the API filters it with an index.
		eventTypes: { type: "array", key: "eventTypes" },
		// Subscription name, exact or trailing-* prefix.
		subscription: { type: "string", key: "subscription" },
		// Connection code, exact or trailing-* prefix.
		connection: { type: "string", key: "connection" },
		// "stuck": PENDING/QUEUED older than 5 minutes.
		lens: { type: "string", key: "lens" },
	},
	pageSize: 200,
	sortField: "createdAt",
	sortOrder: "desc",
});
const {
	filters,
	pageSize,
	hasActiveFilters,
	clearFilters,
	syncToUrl,
	withSuppressed,
	sortOrder,
	onSort,
} = listState;

// Date-range picker models. DatePicker works in Dates; the URL-synced filter
// refs hold YYYY-MM-DD strings — keep both in lockstep (filters are the
// source of truth so deep links and clear-all behave).
function parseDateFilter(v: string): Date | null {
	if (!v) return null;
	const d = new Date(`${v}T00:00:00`);
	return Number.isNaN(d.getTime()) ? null : d;
}
function toDateFilter(d: Date | null): string {
	if (!d) return "";
	const pad = (n: number) => String(n).padStart(2, "0");
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
const dateFrom = ref<Date | null>(parseDateFilter(filters.from.value));
const dateTo = ref<Date | null>(parseDateFilter(filters.to.value));
watch([filters.from, filters.to], ([from, to]) => {
	if (toDateFilter(dateFrom.value) !== from) dateFrom.value = parseDateFilter(from);
	if (toDateFilter(dateTo.value) !== to) dateTo.value = parseDateFilter(to);
});
function onDateRangeChange() {
	// Custom dates replace the preset.
	withSuppressed(() => {
		filters.window.value = "";
		filters.from.value = toDateFilter(dateFrom.value);
		filters.to.value = toDateFilter(dateTo.value);
	});
	syncToUrl();
	load();
}

// ── window ──────────────────────────────────────────────────────────────
// The server holds the list to a created_at window and echoes the one it
// applied (X-Window-* headers). The picker is always visible.
const windowPreset = computed(() => normalizePreset(filters.window.value));
const customWindow = computed(() => !!filters.from.value || !!filters.to.value);
const windowButtons = WINDOW_PRESETS.map((p) => ({ label: p.label.replace("Last ", ""), value: p.key, title: p.label }));
const appliedWindow = ref<WindowResponse | null>(null);
const windowLabel = computed(() => (appliedWindow.value ? describeWindow(appliedWindow.value, new Date()) : null));

function onPresetChange(key: string | null) {
	if (!key) return;
	const sel = selectPreset(key);
	withSuppressed(() => {
		filters.window.value = sel.preset === DEFAULT_WINDOW_KEY ? "" : sel.preset;
		filters.from.value = "";
		filters.to.value = "";
	});
	dateFrom.value = null;
	dateTo.value = null;
	syncToUrl();
	load();
}

// DataTable sort events (server-side; only createdAt is sortable).
function onSortChange(event: { sortField?: unknown; sortOrder?: number | null }) {
	onSort(event as Parameters<typeof onSort>[0]);
	syncToUrl();
	load();
}

// Server-side filtering: the DataTable filter meta isn't bound — popup
// inputs write the listState refs directly and load() serializes them
// into API params. Only the badge count is derived here.
const { activeFilterCount } = useTableFilters(
	listState,
	[
		{ field: "clientId", param: "clients" },
		{ field: "application", param: "applications" },
		{ field: "subdomain", param: "subdomains" },
		{ field: "aggregate", param: "aggregates" },
		{ field: "code", param: "codes" },
		{ field: "status", param: "statuses" },
	],
	{ globalParam: "search" },
);

function currentFilters(): JobListFilters {
	return {
		clients: filters.clients.value,
		applications: filters.applications.value,
		subdomains: filters.subdomains.value,
		aggregates: filters.aggregates.value,
		codes: filters.codes.value,
		statuses: filters.statuses.value,
		search: filters.search.value,
		messageGroup: filters.messageGroup.value,
		eventTypes: filters.eventTypes.value,
		subscription: filters.subscription.value,
		connection: filters.connection.value,
		window: filters.window.value,
		from: filters.from.value,
		to: filters.to.value,
		lens: filters.lens.value,
	};
}

function buildParams(): DispatchJobsListParams {
	return filtersToParams(currentFilters(), {
		size: pageSize.value,
		sort: sortOrder.value === "asc" ? "createdAt.asc" : "createdAt.desc",
		now: new Date(),
	});
}

const dispatchJobs = ref<DispatchJob[]>([]);
const loading = ref(false);
// Bound to the DataTable's multiple-selection checkboxes (dataKey="id").
const selectedJobs = ref<DispatchJob[]>([]);
const requeuing = ref(false);

// The filter the rows on screen were loaded with — "all matching" means
// this, not whatever the filter popup holds while a reload is pending.
const loadedFilter = ref<DispatchJobFilterBody>({});
// The filter summary shown in the confirm dialog, captured with loadedFilter.
const loadedSummary = ref<FilterSummaryItem[]>([]);
// How many jobs loadedFilter matches (preview count); null = not counted yet.
const matchCount = ref<number | null>(null);
// Gmail-style "select all M matching" taken.
const allMatchingSelected = ref(false);
let loadSeq = 0;

const { getLabel: clientLabel } = useClientOptions();

function currentFilterValues() {
	return {
		eventTypes: filters.eventTypes.value,
		subscription: filters.subscription.value,
		connection: filters.connection.value,
		windowLabel:
			!customWindow.value && windowPreset.value !== DEFAULT_WINDOW_KEY
				? WINDOW_PRESETS.find((p) => p.key === windowPreset.value)?.label
				: undefined,
		clients: filters.clients.value,
		applications: filters.applications.value,
		subdomains: filters.subdomains.value,
		aggregates: filters.aggregates.value,
		codes: filters.codes.value,
		statuses: filters.statuses.value,
		search: filters.search.value,
		messageGroup: filters.messageGroup.value,
		from: filters.from.value,
		to: filters.to.value,
	};
}

// A readable message when the API (or the form) refused the filters.
const listError = ref<string | null>(null);

async function load() {
	const mine = ++loadSeq;
	const invalid =
		validateFilters(currentFilters()) ??
		(customRangeTooLarge(currentFilters()) ? explainListError({ code: "WINDOW_TOO_LARGE" }) : null);
	if (invalid) {
		listError.value = invalid;
		dispatchJobs.value = [];
		appliedWindow.value = null;
		return;
	}
	const params = buildParams();
	loading.value = true;
	try {
		const { rows, window } = await dispatchJobsApi.listWithWindow(params);
		if (mine !== loadSeq) return;
		listError.value = null;
		appliedWindow.value = window;
		dispatchJobs.value = rows;
		loadedFilter.value = filterBody(params);
		loadedSummary.value = summarizeFilters(currentFilterValues(), clientLabel);
		matchCount.value = null;
		allMatchingSelected.value = false;
		// Drop selections that are no longer in the refreshed view.
		const visible = new Set(dispatchJobs.value.map((j) => j.id));
		selectedJobs.value = selectedJobs.value.filter((j) => visible.has(j.id));
	} catch (error) {
		if (mine !== loadSeq) return;
		console.error("Failed to load dispatch jobs:", error);
		listError.value = explainListError(error);
		dispatchJobs.value = [];
		appliedWindow.value = null;
	} finally {
		if (mine === loadSeq) loading.value = false;
	}
}

const bannerState = computed(() =>
	selectAllBannerState({
		pageRows: dispatchJobs.value.length,
		selectedRows: selectedJobs.value.length,
		pageSize: pageSize.value,
		matchCount: matchCount.value,
		allMatchingSelected: allMatchingSelected.value,
	}),
);

// The banner offers "select all M matching" — count M once per load, when
// it shows. (matchCount is reset by every load, which re-triggers this.)
let countedForLoad = -1;
watch([bannerState, matchCount], async ([state, count]) => {
	if (state !== "offer" || count !== null || countedForLoad === loadSeq) return;
	const mine = (countedForLoad = loadSeq);
	try {
		const p = await dispatchJobsApi.previewRequeueByFilter(loadedFilter.value);
		if (mine === loadSeq) matchCount.value = p.count;
	} catch {
		// apiFetch has already shown the error; the banner keeps offering.
	}
});

function selectAllMatching() {
	allMatchingSelected.value = true;
}

// Unticking any row leaves "all matching" mode.
watch(
	() => selectedJobs.value.length,
	(n) => {
		if (n !== dispatchJobs.value.length) allMatchingSelected.value = false;
	},
);

function clearSelection() {
	selectedJobs.value = [];
	allMatchingSelected.value = false;
}

// ── requeue all matching ─────────────────────────────────────────────────
const filterDialogVisible = ref(false);

function openRequeueAllMatching() {
	filterDialogVisible.value = true;
}

function onPreviewed(p: RequeueByFilterPreviewResponse) {
	matchCount.value = p.count;
}

async function onFilterRequeueDone(res: RequeueByFilterResponse | null) {
	if (res) {
		const o = requeueOutcome(res);
		notify({ severity: o.severity, summary: o.summary, detail: o.detail, life: o.severity === "warn" ? undefined : 5000 });
	}
	clearSelection();
	await load();
}

// "Requeue everything in error": every job in an error status, whatever
// else the view was filtered by, through the same preview + confirm.
async function requeueEverythingInError() {
	// Every other filter goes; the time window stays (the run is held to it).
	const keep = { window: filters.window.value, from: filters.from.value, to: filters.to.value };
	clearFilters();
	filters.window.value = keep.window;
	filters.from.value = keep.from;
	filters.to.value = keep.to;
	filters.statuses.value = [...ERROR_STATUSES];
	syncToUrl();
	await load();
	filterDialogVisible.value = true;
}

const requeueAllMenu = [
	{ label: "Requeue everything in error in this window", icon: "pi pi-exclamation-circle", command: () => void requeueEverythingInError() },
];

const requeueAllLabel = computed(() =>
	matchCount.value === null
		? "Requeue all matching"
		: `Requeue all matching (${matchCount.value.toLocaleString()})`,
);

// The status filter offers what exists plus whatever is selected (the error
// preset selects ERROR, which may never have been stored).
const statusSelectOptions = computed<FilterOption[]>(() => {
	const known = new Set(statusOptions.value.map((o) => o.value));
	return [
		...statusOptions.value,
		...filters.statuses.value.filter((s) => !known.has(s)).map((s) => ({ label: s, value: s })),
	];
});

// Reset the given jobs to PENDING so the scheduler re-dispatches them. The
// server tenant-scopes the reset and leaves in-flight and COMPLETED jobs
// alone (reported in the response), so `requeued` may be < ids.length.
async function requeueIds(ids: string[]) {
	if (!ids.length || requeuing.value) return;
	requeuing.value = true;
	try {
		showIdsOutcome(await dispatchJobsApi.requeue(ids));
		selectedJobs.value = [];
		await load();
	} catch (error) {
		toast.error(
			"Requeue failed",
			error instanceof Error ? error.message : undefined,
		);
	} finally {
		requeuing.value = false;
	}
}

function showIdsOutcome(res: RequeueResponse) {
	const o = requeueIdsOutcome(res);
	notify({ severity: o.severity, summary: o.summary, detail: o.detail, life: 5000 });
}

// COMPLETED rows among those ticked go through the redrive dialog, but only
// for users who may redrive; everyone else requeues straight away and the
// server reports the COMPLETED ones as left alone.
const selectedDialogVisible = ref(false);
const selectedDialogIds = ref<string[]>([]);
const selectedDialogCompleted = ref(0);

function openSelectedDialog(jobs: DispatchJob[]) {
	selectedDialogIds.value = jobs.map((j) => j.id).filter((id): id is string => !!id);
	selectedDialogCompleted.value = jobs.filter((j) => j.status === "COMPLETED").length;
	selectedDialogVisible.value = true;
}

async function onSelectedDialogDone(res: RequeueResponse | null) {
	if (res) showIdsOutcome(res);
	selectedJobs.value = [];
	await load();
}

function requeueJobs(jobs: DispatchJob[]) {
	if (canRedrive.value && jobs.some((j) => j.status === "COMPLETED")) {
		openSelectedDialog(jobs);
		return;
	}
	void requeueIds(jobs.map((j) => j.id).filter((id): id is string => !!id));
}

function requeueSelected() {
	if (allMatchingSelected.value) {
		openRequeueAllMatching();
		return;
	}
	requeueJobs(selectedJobs.value);
}

const requeueSelectedLabel = computed(() => {
	if (allMatchingSelected.value && matchCount.value !== null) {
		return `Requeue selected (${matchCount.value.toLocaleString()})`;
	}
	return selectedJobs.value.length ? `Requeue selected (${selectedJobs.value.length})` : "Requeue selected";
});

function requeueOne(job: DispatchJob) {
	if (job.id) requeueJobs([job]);
}

// Per-row action availability: permission AND the status the API would accept.
function rowActions(job: DispatchJob) {
	return jobActions(job.status, (p) => userHasPermission(authStore.user, p));
}

// Search / group reload: debounced, replacing the old Enter-to-search.
let searchTimer: ReturnType<typeof setTimeout> | undefined;
watch([filters.search, filters.messageGroup, filters.subscription, filters.connection], () => {
	clearTimeout(searchTimer);
	searchTimer = setTimeout(load, 400);
});

// Row click opens the detail drawer (child route); the list stays mounted
// underneath with its filters intact.
function viewJob(job: DispatchJob) {
	if (job.id) void router.push({ path: `/dispatch-jobs/${job.id}`, query: route.query });
}

// "Additional data" on the grid: the job's key/value metadata, clipped.
function metadataPreview(job: DispatchJob, max = 20): string {
	const meta = job.metadata ?? [];
	if (!meta.length) return "";
	const s = meta.map((m) => `${m.key}=${m.value}`).join(", ");
	return s.length > max ? `${s.slice(0, max)}…` : s;
}
function metadataFull(job: DispatchJob): string {
	return (job.metadata ?? []).map((m) => `${m.key}: ${m.value}`).join("\n");
}

// Filter options. The endpoint returns the distinct codes; the application,
// subdomain and aggregate options are their segments, each narrowed by the
// levels chosen above it (utils/code-facets.ts).
const distinctCodes = ref<string[]>([]);
const facetEventTypes = ref<string[]>([]);
const statusOptions = ref<FilterOption[]>([]);
const toOptions = (values: string[]): FilterOption[] =>
	values.map((v) => ({ label: v, value: v }));
const levels = computed(() =>
	codeLevels(
		distinctCodes.value,
		filters.applications.value,
		filters.subdomains.value,
		filters.aggregates.value,
	),
);
const applicationOptions = computed(() => toOptions(levels.value.applications));
const subdomainOptions = computed(() => toOptions(levels.value.subdomains));
const aggregateOptions = computed(() => toOptions(levels.value.aggregates));
const codeOptions = computed(() => toOptions(levels.value.codes));

onMounted(async () => {
	await loadFilterOptions();
	await load();
});

async function loadFilterOptions() {
	try {
		const data = await dispatchJobsApi.filterOptions();
		// The wire facets are plain string arrays (statuses/codes/clientIds/
		// dispatchPoolIds/subscriptionIds/kinds); the application, subdomain
		// and aggregate options are derived from the codes (above).
		distinctCodes.value = data.codes;
		facetEventTypes.value = data.eventTypes ?? data.codes;
		statusOptions.value = toOptions(data.statuses);
	} catch (error) {
		console.error("Failed to load filter options:", error);
	}
}

// ── quick lenses ────────────────────────────────────────────────────────
// Failed = FAILED/ERROR; Stuck = PENDING/QUEUED created more than 5 minutes
// ago. They set the status filter (visible in the Status popup) over the
// other filters; choosing the active lens again clears it.
const lens = computed(() => activeLens({ statuses: filters.statuses.value, lens: filters.lens.value }));
function toggleLens(l: Lens) {
	withSuppressed(() => {
		if (lens.value === l) {
			filters.statuses.value = [];
			filters.lens.value = "";
		} else {
			const next = lensFilters(l);
			filters.statuses.value = next.statuses;
			filters.lens.value = next.lens;
		}
	});
	syncToUrl();
	load();
}

// Picking statuses by hand leaves the Stuck cap behind.
function onStatusesChange() {
	if (lens.value !== "stuck" && filters.lens.value) {
		withSuppressed(() => {
			filters.lens.value = "";
		});
		syncToUrl();
	}
	load();
}

// ── event type ──────────────────────────────────────────────────────────
// Suggestions come from the window's distinct codes (a job's code is its
// event type); anything typed is accepted too.
const eventTypeSuggestions = ref<string[]>([]);
function searchEventTypes(e: { query: string }) {
	eventTypeSuggestions.value = suggestEventTypes(e.query, facetEventTypes.value, filters.eventTypes.value);
}
const eventTypesError = computed(() => validateFilters({ eventTypes: filters.eventTypes.value }));

// Cascading clears: changing a parent wipes its dependent children. The
// child writes are wrapped in `withSuppressed` so useListState's per-ref
// watchers don't each spam syncToUrl; we sync once at the end.
function onClientsChange() {
	withSuppressed(() => {
		filters.applications.value = [];
		filters.subdomains.value = [];
		filters.aggregates.value = [];
		filters.codes.value = [];
	});
	syncToUrl();
	load();
}

function onApplicationsChange() {
	withSuppressed(() => {
		filters.subdomains.value = [];
		filters.aggregates.value = [];
		filters.codes.value = [];
	});
	syncToUrl();
	load();
}

function onSubdomainsChange() {
	withSuppressed(() => {
		filters.aggregates.value = [];
		filters.codes.value = [];
	});
	syncToUrl();
	load();
}

function onAggregatesChange() {
	withSuppressed(() => {
		filters.codes.value = [];
	});
	syncToUrl();
	load();
}

function clearAllFilters() {
	clearFilters();
	// The date pickers follow filters.from/to through a watcher that runs
	// after this; load() reads them now.
	dateFrom.value = null;
	dateTo.value = null;
	load();
}

function formatDate(dateStr: string | undefined): string {
	if (!dateStr) return "-";
	return new Date(dateStr).toLocaleString();
}

function formatCode(code: string | undefined): {
	app?: string;
	subdomain?: string;
	aggregate?: string;
	event?: string;
} {
	if (!code) return {};
	const parts = code.split(":");
	return {
		app: parts[0],
		subdomain: parts[1],
		aggregate: parts[2],
		event: parts[3],
	};
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Dispatch Jobs</h1>
        <p class="page-subtitle">Monitor webhook dispatch jobs and delivery status</p>
      </div>
    </header>

    <div class="window-bar" role="group" aria-label="Time window">
      <span class="window-title">Window</span>
      <SelectButton
        :modelValue="customWindow ? null : windowPreset"
        :options="windowButtons"
        optionLabel="label"
        optionValue="value"
        :allowEmpty="false"
        size="small"
        aria-label="Window preset"
        @update:modelValue="onPresetChange"
      />
      <span class="window-or">or</span>
      <DatePicker
        v-model="dateFrom"
        dateFormat="yy-mm-dd"
        placeholder="From"
        showIcon
        showButtonBar
        size="small"
        :maxDate="dateTo ?? undefined"
        aria-label="Window from"
        @update:modelValue="onDateRangeChange"
      />
      <DatePicker
        v-model="dateTo"
        dateFormat="yy-mm-dd"
        placeholder="To"
        showIcon
        showButtonBar
        size="small"
        :minDate="dateFrom ?? undefined"
        aria-label="Window to"
        @update:modelValue="onDateRangeChange"
      />
      <span v-if="windowLabel" class="window-label" data-testid="window-label">
        {{ windowLabel.text }}
        <Tag v-if="windowLabel.defaulted" value="default window" severity="secondary" />
      </span>
    </div>
    <p v-if="windowLabel?.defaulted" class="window-hint">
      No dates were chosen, so only the last 24 hours are shown. Pick a longer window to look further back (up to 31 days).
    </p>

    <div class="lens-bar" role="group" aria-label="Quick views">
      <span class="window-title">Quick views</span>
      <Button
        label="Failed"
        icon="pi pi-times-circle"
        size="small"
        :severity="lens === 'failed' ? 'danger' : 'secondary'"
        :outlined="lens !== 'failed'"
        :aria-pressed="lens === 'failed'"
        v-tooltip="'FAILED and ERROR jobs'"
        @click="toggleLens('failed')"
      />
      <Button
        label="Stuck"
        icon="pi pi-clock"
        size="small"
        :severity="lens === 'stuck' ? 'warn' : 'secondary'"
        :outlined="lens !== 'stuck'"
        :aria-pressed="lens === 'stuck'"
        v-tooltip="'PENDING or QUEUED jobs created more than 5 minutes ago'"
        @click="toggleLens('stuck')"
      />
    </div>

    <Message v-if="listError" severity="error" :closable="false" class="list-error">{{ listError }}</Message>

    <div class="fc-card">
      <DataTable
        v-model:selection="selectedJobs"
        dataKey="id"
        :value="dispatchJobs"
        :loading="loading"
        stripedRows
        lazy
        sortField="createdAt"
        :sortOrder="sortOrder === 'asc' ? 1 : -1"
        emptyMessage="No dispatch jobs found"
        tableStyle="min-width: 60rem"
        @sort="onSortChange"
      >
        <template #header>
          <FcTableToolbar
            v-model:search="filters.search.value"
            search-placeholder="Search by source..."
            :active-filter-count="activeFilterCount + (filters.eventTypes.value.length ? 1 : 0) + (filters.subscription.value ? 1 : 0) + (filters.connection.value ? 1 : 0) + (filters.messageGroup.value ? 1 : 0)"
            :has-active-filters="hasActiveFilters"
            show-refresh
            @refresh="load"
            @clear-all="clearAllFilters"
          >
            <template #actions>
              <Button
                v-if="canRequeue"
                :label="requeueSelectedLabel"
                icon="pi pi-replay"
                size="small"
                severity="secondary"
                :disabled="!selectedJobs.length || requeuing"
                :loading="requeuing"
                @click="requeueSelected"
                v-tooltip="'Reset the selected jobs to PENDING for re-dispatch'"
              />
              <SplitButton
                v-if="canRequeue"
                :label="requeueAllLabel"
                icon="pi pi-replay"
                size="small"
                severity="secondary"
                outlined
                :model="requeueAllMenu"
                :disabled="loading || requeuing"
                @click="openRequeueAllMatching"
                v-tooltip.top="'Reset every job matching the current filters to PENDING — preview and confirm first'"
              />
              <Select
                v-model="pageSize"
                :options="sizeOptions"
                class="size-select"
                @change="load"
                v-tooltip="'Result size — most recent N jobs'"
              />
            </template>
            <template #filters>
              <FcFormField label="Client">
                <ClientFilter
                  v-model="filters.clients.value"
                  appendTo="self"
                  @change="onClientsChange"
                />
              </FcFormField>
              <FcFormField label="Application">
                <template #default="{ id: fieldId }">
                  <MultiSelect
                    :id="fieldId"
                    v-model="filters.applications.value"
                    :options="applicationOptions"
                    optionLabel="label"
                    optionValue="value"
                    placeholder="All Applications"
                    appendTo="self"
                    @change="onApplicationsChange"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Subdomain">
                <template #default="{ id: fieldId }">
                  <MultiSelect
                    :id="fieldId"
                    v-model="filters.subdomains.value"
                    :options="subdomainOptions"
                    optionLabel="label"
                    optionValue="value"
                    placeholder="All Subdomains"
                    appendTo="self"
                    @change="onSubdomainsChange"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Aggregate">
                <template #default="{ id: fieldId }">
                  <MultiSelect
                    :id="fieldId"
                    v-model="filters.aggregates.value"
                    :options="aggregateOptions"
                    optionLabel="label"
                    optionValue="value"
                    placeholder="All Aggregates"
                    appendTo="self"
                    @change="onAggregatesChange"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Code">
                <template #default="{ id: fieldId }">
                  <MultiSelect
                    :id="fieldId"
                    v-model="filters.codes.value"
                    :options="codeOptions"
                    optionLabel="label"
                    optionValue="value"
                    placeholder="All Codes"
                    appendTo="self"
                    @change="load"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Status">
                <template #default="{ id: fieldId }">
                  <MultiSelect
                    :id="fieldId"
                    v-model="filters.statuses.value"
                    :options="statusSelectOptions"
                    optionLabel="label"
                    optionValue="value"
                    placeholder="All Statuses"
                    appendTo="self"
                    @change="onStatusesChange"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Message group">
                <template #default="{ id: fieldId }">
                  <InputText
                    :id="fieldId"
                    v-model="filters.messageGroup.value"
                    placeholder="Exact message group"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Event type">
                <template #default="{ id: fieldId }">
                  <AutoComplete
                    :inputId="fieldId"
                    v-model="filters.eventTypes.value"
                    multiple
                    dropdown
                    :suggestions="eventTypeSuggestions"
                    :autoOptionFocus="true"
                    :invalid="!!eventTypesError"
                    placeholder="Type or pick an event type"
                    appendTo="self"
                    @complete="searchEventTypes"
                    @update:modelValue="load"
                  />
                  <small :class="eventTypesError ? 'p-error' : 'field-hint'">
                    {{ eventTypesError ?? `Up to ${MAX_EVENT_TYPES}. Press Enter to add what you typed.` }}
                  </small>
                </template>
              </FcFormField>
              <FcFormField label="Subscription">
                <template #default="{ id: fieldId }">
                  <InputText
                    :id="fieldId"
                    v-model="filters.subscription.value"
                    placeholder="Subscription name"
                  />
                  <small class="field-hint">Exact name (any case), or end with * for a prefix: orders*</small>
                </template>
              </FcFormField>
              <FcFormField label="Connection">
                <template #default="{ id: fieldId }">
                  <InputText
                    :id="fieldId"
                    v-model="filters.connection.value"
                    placeholder="Connection code"
                  />
                  <small class="field-hint">Exact code, or end with * for a prefix: billing*</small>
                </template>
              </FcFormField>
            </template>
          </FcTableToolbar>
          <div v-if="bannerState !== 'hidden'" class="select-all-banner" role="status">
            <template v-if="bannerState === 'offer'">
              All {{ dispatchJobs.length.toLocaleString() }} jobs on this page are selected.
              <Button
                link
                size="small"
                class="banner-link"
                :label="matchCount === null ? 'Select all matching jobs…' : `Select all ${matchCount.toLocaleString()} matching jobs`"
                :disabled="matchCount === null"
                @click="selectAllMatching"
              />
            </template>
            <template v-else>
              All {{ matchCount === null ? '' : matchCount.toLocaleString() + ' ' }}jobs matching the current filters are selected.
              <Button link size="small" class="banner-link" label="Clear selection" @click="clearSelection" />
            </template>
          </div>
        </template>

        <Column selectionMode="multiple" headerStyle="width: 3rem" />
        <Column field="id" header="Job ID" style="width: 11rem">
          <template #body="{ data }">
            <a class="font-mono text-sm row-link" @click.prevent="viewJob(data)">{{ data.id }}</a>
          </template>
        </Column>
        <Column field="descriptor" header="Descriptor">
          <template #body="{ data }">
            <span v-if="data.descriptor" class="text-sm">{{ data.descriptor }}</span>
            <span v-else class="text-sm text-muted">-</span>
          </template>
        </Column>
        <Column field="code" header="Code">
          <template #body="{ data }">
            <span class="code-display">
              <span class="code-segment app">{{ formatCode(data.code).app }}</span>
              <span class="code-separator">:</span>
              <span class="code-segment subdomain">{{ formatCode(data.code).subdomain }}</span>
              <span class="code-separator">:</span>
              <span class="code-segment aggregate">{{ formatCode(data.code).aggregate }}</span>
              <span class="code-separator">:</span>
              <span class="code-segment event">{{ formatCode(data.code).event }}</span>
            </span>
          </template>
        </Column>
        <Column field="clientIdentifier" header="Client" style="width: 10rem">
          <template #body="{ data }">
            <span v-if="data.clientIdentifier" class="text-sm">{{ data.clientIdentifier }}</span>
            <span v-else-if="data.clientId" class="font-mono text-sm" v-tooltip="'Client id (identifier not resolved)'">{{ data.clientId }}</span>
            <span v-else class="text-sm text-muted">platform</span>
          </template>
        </Column>
        <Column field="messageGroup" header="Group" style="width: 9rem">
          <template #body="{ data }">
            <a
              v-if="data.messageGroup"
              class="font-mono text-sm row-link truncate"
              style="max-width: 8rem; display: inline-block"
              v-tooltip="`Filter by group ${data.messageGroup}`"
              @click.prevent="filters.messageGroup.value = data.messageGroup"
            >{{ data.messageGroup }}</a>
            <span v-else class="text-sm text-muted">-</span>
          </template>
        </Column>
        <Column field="status" header="Status" style="width: 8rem">
          <template #body="{ data }">
            <Tag :value="data.status" :severity="statusSeverity(data.status)" />
          </template>
        </Column>
        <Column header="Additional data" style="width: 12rem">
          <template #body="{ data }">
            <span v-if="metadataPreview(data)" class="text-sm font-mono" v-tooltip="metadataFull(data)">{{ metadataPreview(data) }}</span>
            <span v-else class="text-sm text-muted">-</span>
          </template>
        </Column>
        <Column field="createdAt" header="Created" sortable style="width: 10rem">
          <template #body="{ data }">
            <span class="text-sm">{{ formatDate(data.createdAt) }}</span>
          </template>
        </Column>
        <Column header="Actions" style="width: 7rem">
          <template #body="{ data }">
            <div class="action-buttons">
              <Button
                icon="pi pi-eye"
                text
                rounded
                size="small"
                v-tooltip="'View payload and attempts'"
                @click="viewJob(data)"
              />
              <Button
                v-if="rowActions(data).requeue"
                icon="pi pi-replay"
                text
                rounded
                size="small"
                v-tooltip="'Requeue — reset to PENDING for re-dispatch'"
                :disabled="requeuing"
                @click="requeueOne(data)"
              />
              <Button
                v-if="rowActions(data).redrive"
                icon="pi pi-exclamation-triangle"
                text
                rounded
                size="small"
                severity="danger"
                v-tooltip="'Redrive — deliver this COMPLETED job again (asks you to confirm twice)'"
                :disabled="requeuing"
                @click="requeueOne(data)"
              />
            </div>
          </template>
        </Column>
      </DataTable>

      <!-- No pagination — dispatch jobs ingest at high rates and "page 2"
           is meaningless. Adjust size or narrow filters to see more. -->
      <div class="result-summary">
        Showing {{ dispatchJobs.length }} dispatch jobs
        ({{ sortOrder === 'asc' ? 'oldest' : 'newest' }} first)
        <span v-if="dispatchJobs.length === pageSize"> (size limit reached — narrow filters or increase size)</span>
      </div>
    </div>

    <RequeueByFilterDialog
      v-model:visible="filterDialogVisible"
      :filter="loadedFilter"
      :summary="loadedSummary"
      @previewed="onPreviewed"
      @done="onFilterRequeueDone"
    />

    <RequeueSelectedDialog
      v-model:visible="selectedDialogVisible"
      :ids="selectedDialogIds"
      :completed-count="selectedDialogCompleted"
      @done="onSelectedDialogDone"
    />

    <!-- Detail drawer (child route /dispatch-jobs/:id) -->
    <RouterView />
  </div>
</template>

<style scoped>
.window-bar,
.lens-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.window-title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--text-color-secondary);
  min-width: 5.5rem;
}

.window-or {
  font-size: 0.8125rem;
  color: var(--text-color-secondary);
}

.window-label {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  margin-left: 0.5rem;
  font-size: 0.875rem;
}

.window-hint {
  margin: 0 0 0.75rem;
  font-size: 0.8125rem;
  color: var(--text-color-secondary);
}

.list-error {
  margin-bottom: 0.75rem;
}

.field-hint {
  color: var(--text-color-secondary);
}

.size-select {
  width: 6rem;
}

.select-all-banner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  margin-top: 0.5rem;
  padding: 0.375rem 0.75rem;
  border: 1px solid var(--surface-border);
  border-radius: 6px;
  font-size: 0.875rem;
  background: var(--surface-ground);
  color: var(--text-color);
}

.banner-link {
  padding: 0 0.25rem;
}

.result-summary {
  text-align: center;
  font-size: 0.8125rem;
  color: var(--text-color-secondary);
  padding: 0.75rem 0 0.25rem;
}

.font-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.text-sm {
  font-size: 0.875rem;
}

.truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.action-buttons {
  display: flex;
  gap: 0.25rem;
  align-items: center;
}

.row-link {
  cursor: pointer;
  color: var(--primary-color);
}
.row-link:hover {
  text-decoration: underline;
}

.text-muted {
  color: var(--text-color-secondary);
}
</style>
