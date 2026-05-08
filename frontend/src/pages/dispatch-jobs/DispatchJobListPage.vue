<script setup lang="ts">
import { ref, onMounted } from "vue";
import ClientFilter from "@/components/ClientFilter.vue";
import { useCursorPagination } from "@/composables/useCursorPagination";
import {
	dispatchJobsApi,
	type DispatchJobRead as DispatchJob,
	type DispatchJobsListParams,
} from "@/api/dispatch-jobs";

interface FilterOption {
	label: string;
	value: string;
}

const filters = {
	clients: ref<string[]>([]),
	applications: ref<string[]>([]),
	subdomains: ref<string[]>([]),
	aggregates: ref<string[]>([]),
	codes: ref<string[]>([]),
	statuses: ref<string[]>([]),
	search: ref<string>(""),
};
const pageSize = ref(100);

function buildParams(after: string | undefined): DispatchJobsListParams {
	return {
		after,
		size: pageSize.value,
		clientIds: filters.clients.value.length ? filters.clients.value : undefined,
		statuses: filters.statuses.value.length ? filters.statuses.value : undefined,
		applications: filters.applications.value.length ? filters.applications.value : undefined,
		subdomains: filters.subdomains.value.length ? filters.subdomains.value : undefined,
		aggregates: filters.aggregates.value.length ? filters.aggregates.value : undefined,
		codes: filters.codes.value.length ? filters.codes.value : undefined,
		source: filters.search.value || undefined,
	};
}

const cursor = useCursorPagination<DispatchJob>({
	fetchPage: (after) => dispatchJobsApi.list(buildParams(after)),
});
const dispatchJobs = cursor.items;
const loading = cursor.loading;

// Filter options
const applicationOptions = ref<FilterOption[]>([]);
const subdomainOptions = ref<FilterOption[]>([]);
const aggregateOptions = ref<FilterOption[]>([]);
const codeOptions = ref<FilterOption[]>([]);
const statusOptions = ref<FilterOption[]>([]);

// Prevent infinite loops from cascading updates
const isUpdating = ref(false);

onMounted(async () => {
	await loadFilterOptions();
	await cursor.loadFirst();
});

async function loadFilterOptions() {
	try {
		const data = await dispatchJobsApi.filterOptions();
		applicationOptions.value = data.applications || [];
		subdomainOptions.value = data.subdomains || [];
		aggregateOptions.value = data.aggregates || [];
		codeOptions.value = data.codes || [];
		statusOptions.value = data.statuses || [];
	} catch (error) {
		console.error("Failed to load filter options:", error);
	}
}

async function onFilterChange(
	clearDownstream:
		| "applications"
		| "subdomains"
		| "aggregates"
		| "codes"
		| "none" = "none",
) {
	if (isUpdating.value) return;
	isUpdating.value = true;
	try {
		if (clearDownstream === "applications") {
			filters.applications.value = [];
			filters.subdomains.value = [];
			filters.aggregates.value = [];
			filters.codes.value = [];
		} else if (clearDownstream === "subdomains") {
			filters.subdomains.value = [];
			filters.aggregates.value = [];
			filters.codes.value = [];
		} else if (clearDownstream === "aggregates") {
			filters.aggregates.value = [];
			filters.codes.value = [];
		} else if (clearDownstream === "codes") {
			filters.codes.value = [];
		}

		await loadFilterOptions();
		await cursor.reset();
	} finally {
		isUpdating.value = false;
	}
}

async function onStatusChange() {
	await cursor.reset();
}

async function onSearchChange() {
	await cursor.reset();
}

function getSeverity(
	status: string,
):
	| "success"
	| "info"
	| "warn"
	| "danger"
	| "secondary"
	| "contrast"
	| undefined {
	switch (status) {
		case "COMPLETED":
			return "success";
		case "PENDING":
			return "info";
		case "QUEUED":
			return "info";
		case "PROCESSING":
			return "warn";
		case "FAILED":
			return "danger";
		case "CANCELLED":
			return "secondary";
		case "EXPIRED":
			return "secondary";
		default:
			return "secondary";
	}
}

function getModeSeverity(
	mode: string,
):
	| "success"
	| "info"
	| "warn"
	| "danger"
	| "secondary"
	| "contrast"
	| undefined {
	switch (mode) {
		case "IMMEDIATE":
			return "success";
		case "NEXT_ON_ERROR":
			return "warn";
		case "BLOCK_ON_ERROR":
			return "danger";
		default:
			return "secondary";
	}
}

function formatDate(dateStr: string | undefined): string {
	if (!dateStr) return "-";
	return new Date(dateStr).toLocaleString();
}

function formatAttempts(job: DispatchJob): string {
	return `${job.attemptCount || 0}/${(job["maxRetries"] as number | undefined) || 3}`;
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
      <div class="toolbar">
        <div class="filter-row">
          <ClientFilter
            v-model="filters.clients.value"
            class="filter-select"
            @change="onFilterChange('applications')"
          />
          <MultiSelect
            v-model="filters.applications.value"
            :options="applicationOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Applications"
            class="filter-select"
            @change="onFilterChange('subdomains')"
          />
          <MultiSelect
            v-model="filters.subdomains.value"
            :options="subdomainOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Subdomains"
            class="filter-select"
            @change="onFilterChange('aggregates')"
          />
          <MultiSelect
            v-model="filters.aggregates.value"
            :options="aggregateOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Aggregates"
            class="filter-select"
            @change="onFilterChange('codes')"
          />
          <MultiSelect
            v-model="filters.codes.value"
            :options="codeOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Codes"
            class="filter-select"
            @change="onFilterChange('none')"
          />
        </div>
        <div class="filter-row">
          <MultiSelect
            v-model="filters.statuses.value"
            :options="statusOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Statuses"
            class="filter-select"
            @change="onStatusChange"
          />
          <IconField>
            <InputIcon class="pi pi-search" />
            <InputText
              v-model="filters.search.value"
              placeholder="Search by source..."
              @keyup.enter="onSearchChange"
            />
          </IconField>
          <Button
            icon="pi pi-refresh"
            text
            rounded
            @click="cursor.refresh"
            v-tooltip="'Refresh'"
          />
        </div>
      </div>

      <DataTable
        :value="dispatchJobs"
        :loading="loading"
        stripedRows
        emptyMessage="No dispatch jobs found"
        tableStyle="min-width: 60rem"
      >
        <Column field="id" header="Job ID" style="width: 10rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ data.id?.slice(0, 8) }}...</span>
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
        <Column field="source" header="Source" />
        <Column field="status" header="Status" style="width: 8rem">
          <template #body="{ data }">
            <Tag :value="data.status" :severity="getSeverity(data.status)" />
          </template>
        </Column>
        <Column field="mode" header="Mode" style="width: 8rem">
          <template #body="{ data }">
            <Tag :value="data.mode || 'IMMEDIATE'" :severity="getModeSeverity(data.mode)" />
          </template>
        </Column>
        <Column header="Attempts" style="width: 6rem">
          <template #body="{ data }">
            {{ formatAttempts(data) }}
          </template>
        </Column>
        <Column field="targetUrl" header="Target URL">
          <template #body="{ data }">
            <span class="text-sm truncate" style="max-width: 200px; display: inline-block">
              {{ data.targetUrl }}
            </span>
          </template>
        </Column>
        <Column field="createdAt" header="Created" style="width: 10rem">
          <template #body="{ data }">
            <span class="text-sm">{{ formatDate(data.createdAt) }}</span>
          </template>
        </Column>
        <Column header="Actions" style="width: 8rem">
          <template #body="{ data }">
            <div class="action-buttons">
              <Button icon="pi pi-eye" text rounded size="small" v-tooltip="'View details'" />
              <Button
                icon="pi pi-replay"
                text
                rounded
                size="small"
                v-tooltip="'Retry'"
                :disabled="data.status === 'COMPLETED' || data.status === 'PROCESSING'"
              />
            </div>
          </template>
        </Column>
      </DataTable>

      <!-- Cursor pager. msg_dispatch_jobs is unbounded so we keyset rather
           than count. ← Newer / Older → for in-session navigation. -->
      <div class="cursor-pager">
        <Button
          icon="pi pi-angle-left"
          label="Newer"
          text
          :disabled="!cursor.hasPrev.value || cursor.loading.value"
          @click="cursor.loadPrev"
        />
        <span class="page-indicator">Page {{ cursor.page.value }}</span>
        <Button
          icon="pi pi-angle-right"
          iconPos="right"
          label="Older"
          text
          :disabled="!cursor.hasMore.value || cursor.loading.value"
          @click="cursor.loadNext"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.cursor-pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  padding: 0.75rem 0 0.25rem;
}

.page-indicator {
  font-size: 0.875rem;
  color: var(--text-color-secondary);
  min-width: 4.5rem;
  text-align: center;
}

.toolbar {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-bottom: 16px;
}

.filter-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.filter-select {
  min-width: 160px;
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
</style>
