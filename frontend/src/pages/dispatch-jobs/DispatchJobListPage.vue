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
} from "@/api/dispatch-jobs";
import { notify, toast } from "@/utils/errorBus";
import RequeueByFilterDialog from "./RequeueByFilterDialog.vue";
import {
	ERROR_STATUSES,
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
	filters.from.value = toDateFilter(dateFrom.value);
	filters.to.value = toDateFilter(dateTo.value);
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

function buildParams(): DispatchJobsListParams {
	// The range filter is whole days: since = start of the "from" day,
	// until = end of the "to" day, in the viewer's timezone.
	const since = dateFrom.value
		? new Date(
				dateFrom.value.getFullYear(),
				dateFrom.value.getMonth(),
				dateFrom.value.getDate(),
			).toISOString()
		: undefined;
	const until = dateTo.value
		? new Date(
				dateTo.value.getFullYear(),
				dateTo.value.getMonth(),
				dateTo.value.getDate() + 1,
			).toISOString()
		: undefined;
	return {
		size: pageSize.value,
		clientIds: filters.clients.value.length ? filters.clients.value : undefined,
		statuses: filters.statuses.value.length ? filters.statuses.value : undefined,
		applications: filters.applications.value.length ? filters.applications.value : undefined,
		subdomains: filters.subdomains.value.length ? filters.subdomains.value : undefined,
		aggregates: filters.aggregates.value.length ? filters.aggregates.value : undefined,
		codes: filters.codes.value.length ? filters.codes.value : undefined,
		source: filters.search.value || undefined,
		messageGroup: filters.messageGroup.value || undefined,
		since,
		until,
		sort: sortOrder.value === "asc" ? "createdAt.asc" : "createdAt.desc",
	};
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

async function load() {
	const mine = ++loadSeq;
	const params = buildParams();
	loading.value = true;
	try {
		const rows = await dispatchJobsApi.list(params);
		if (mine !== loadSeq) return;
		dispatchJobs.value = rows;
		loadedFilter.value = filterBody(params);
		loadedSummary.value = summarizeFilters(currentFilterValues(), clientLabel);
		matchCount.value = null;
		allMatchingSelected.value = false;
		// Drop selections that are no longer in the refreshed view.
		const visible = new Set(dispatchJobs.value.map((j) => j.id));
		selectedJobs.value = selectedJobs.value.filter((j) => visible.has(j.id));
	} catch (error) {
		console.error("Failed to load dispatch jobs:", error);
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
	clearFilters();
	dateFrom.value = null;
	dateTo.value = null;
	filters.statuses.value = [...ERROR_STATUSES];
	syncToUrl();
	await load();
	filterDialogVisible.value = true;
}

const requeueAllMenu = [
	{ label: "Requeue everything in error", icon: "pi pi-exclamation-circle", command: () => void requeueEverythingInError() },
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
// server tenant-scopes the reset, so `requeued` may be < ids.length.
async function requeueIds(ids: string[]) {
	if (!ids.length || requeuing.value) return;
	requeuing.value = true;
	try {
		const { requeued } = await dispatchJobsApi.requeue(ids);
		toast.success(
			"Requeued",
			`${requeued} dispatch job${requeued === 1 ? "" : "s"} reset to PENDING`,
		);
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

function requeueSelected() {
	if (allMatchingSelected.value) {
		openRequeueAllMatching();
		return;
	}
	requeueIds(
		selectedJobs.value.map((j) => j.id).filter((id): id is string => !!id),
	);
}

const requeueSelectedLabel = computed(() => {
	if (allMatchingSelected.value && matchCount.value !== null) {
		return `Requeue selected (${matchCount.value.toLocaleString()})`;
	}
	return selectedJobs.value.length ? `Requeue selected (${selectedJobs.value.length})` : "Requeue selected";
});

function requeueOne(job: DispatchJob) {
	if (job.id) requeueIds([job.id]);
}

// Search / group reload: debounced, replacing the old Enter-to-search.
let searchTimer: ReturnType<typeof setTimeout> | undefined;
watch([filters.search, filters.messageGroup], () => {
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

// Filter options
const applicationOptions = ref<FilterOption[]>([]);
const subdomainOptions = ref<FilterOption[]>([]);
const aggregateOptions = ref<FilterOption[]>([]);
const codeOptions = ref<FilterOption[]>([]);
const statusOptions = ref<FilterOption[]>([]);

onMounted(async () => {
	await loadFilterOptions();
	await load();
});

async function loadFilterOptions() {
	try {
		const data = await dispatchJobsApi.filterOptions();
		// The wire facets are plain string arrays (statuses/codes/clientIds/
		// dispatchPoolIds/subscriptionIds/kinds); applications, subdomains and
		// aggregates are not surfaced by this endpoint, so those selects stay
		// empty (they were silently empty before, too — the old shape never
		// matched the wire).
		const toOptions = (values: string[]): FilterOption[] =>
			values.map((v) => ({ label: v, value: v }));
		applicationOptions.value = [];
		subdomainOptions.value = [];
		aggregateOptions.value = [];
		codeOptions.value = toOptions(data.codes);
		statusOptions.value = toOptions(data.statuses);
	} catch (error) {
		console.error("Failed to load filter options:", error);
	}
}

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
            :active-filter-count="activeFilterCount + (filters.from.value ? 1 : 0) + (filters.to.value ? 1 : 0) + (filters.messageGroup.value ? 1 : 0)"
            :has-active-filters="hasActiveFilters"
            show-refresh
            @refresh="load"
            @clear-all="clearAllFilters"
          >
            <template #actions>
              <Button
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
                    @change="load"
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
              <FcFormField label="Created from">
                <template #default="{ id: fieldId }">
                  <DatePicker
                    :id="fieldId"
                    v-model="dateFrom"
                    dateFormat="yy-mm-dd"
                    placeholder="Any date"
                    showIcon
                    showButtonBar
                    appendTo="self"
                    :maxDate="dateTo ?? undefined"
                    @update:modelValue="onDateRangeChange"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Created to">
                <template #default="{ id: fieldId }">
                  <DatePicker
                    :id="fieldId"
                    v-model="dateTo"
                    dateFormat="yy-mm-dd"
                    placeholder="Any date"
                    showIcon
                    showButtonBar
                    appendTo="self"
                    :minDate="dateFrom ?? undefined"
                    @update:modelValue="onDateRangeChange"
                  />
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
                icon="pi pi-replay"
                text
                rounded
                size="small"
                v-tooltip="'Requeue — reset to PENDING for re-dispatch'"
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

    <!-- Detail drawer (child route /dispatch-jobs/:id) -->
    <RouterView />
  </div>
</template>

<style scoped>
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
