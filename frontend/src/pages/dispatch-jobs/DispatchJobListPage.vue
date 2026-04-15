<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useListState } from "@/composables/useListState";
import {
	getApiAdminDispatchJobs,
	getApiAdminDispatchJobsFilterOptions,
} from "@/api/generated";

interface DispatchJob {
	id: string;
	source: string;
	code: string;
	kind: string;
	targetUrl: string;
	status: string;
	mode: string;
	clientId?: string;
	subscriptionId?: string;
	dispatchPoolId?: string;
	attemptCount: number;
	maxRetries: number;
	createdAt: string;
	updatedAt: string;
	completedAt?: string;
	lastError?: string;
}

interface FilterOption {
	label: string;
	value: string;
}

const dispatchJobs = ref<DispatchJob[]>([]);
const loading = ref(true);
const totalRecords = ref(0);

const { filters, page, pageSize, sortField, sortOrder, onPage, onSort } =
	useListState(
		{
			filters: {
				clients: { type: "array", key: "clients" },
				applications: { type: "array", key: "apps" },
				subdomains: { type: "array", key: "subs" },
				aggregates: { type: "array", key: "aggs" },
				codes: { type: "array", key: "codes" },
				statuses: { type: "array", key: "statuses" },
				search: { type: "string", key: "q" },
			},
			pageSize: 100,
			sortField: "createdAt",
			sortOrder: "desc",
		},
		() => loadDispatchJobs(),
	);

// Filter options
const clientOptions = ref<FilterOption[]>([]);
const applicationOptions = ref<FilterOption[]>([]);
const subdomainOptions = ref<FilterOption[]>([]);
const aggregateOptions = ref<FilterOption[]>([]);
const codeOptions = ref<FilterOption[]>([]);
const statusOptions = ref<FilterOption[]>([]);

// Prevent infinite loops from cascading updates
const isUpdating = ref(false);

onMounted(async () => {
	await loadFilterOptions();
	await loadDispatchJobs();
});

async function loadFilterOptions() {
	try {
		const response = await getApiAdminDispatchJobsFilterOptions({
			query: {
				clientIds:
					filters.clients.value.length > 0
						? filters.clients.value.join(",")
						: undefined,
				applications:
					filters.applications.value.length > 0
						? filters.applications.value.join(",")
						: undefined,
				subdomains:
					filters.subdomains.value.length > 0
						? filters.subdomains.value.join(",")
						: undefined,
				aggregates:
					filters.aggregates.value.length > 0
						? filters.aggregates.value.join(",")
						: undefined,
			},
		});
		const data = response.data as unknown as {
			clients?: FilterOption[];
			applications?: FilterOption[];
			subdomains?: FilterOption[];
			aggregates?: FilterOption[];
			codes?: FilterOption[];
			statuses?: FilterOption[];
		};
		if (data) {
			clientOptions.value = (data.clients || []) as FilterOption[];
			applicationOptions.value = (data.applications || []) as FilterOption[];
			subdomainOptions.value = (data.subdomains || []) as FilterOption[];
			aggregateOptions.value = (data.aggregates || []) as FilterOption[];
			codeOptions.value = (data.codes || []) as FilterOption[];
			statusOptions.value = (data.statuses || []) as FilterOption[];
		}
	} catch (error) {
		console.error("Failed to load filter options:", error);
	}
}

async function loadDispatchJobs() {
	loading.value = true;
	try {
		const response = await getApiAdminDispatchJobs({
			query: {
				page: String(page.value),
				size: String(pageSize.value),
				sortField: sortField.value,
				sortOrder: sortOrder.value,
				clientIds:
					filters.clients.value.length > 0
						? filters.clients.value.join(",")
						: undefined,
				statuses:
					filters.statuses.value.length > 0
						? filters.statuses.value.join(",")
						: undefined,
				applications:
					filters.applications.value.length > 0
						? filters.applications.value.join(",")
						: undefined,
				subdomains:
					filters.subdomains.value.length > 0
						? filters.subdomains.value.join(",")
						: undefined,
				aggregates:
					filters.aggregates.value.length > 0
						? filters.aggregates.value.join(",")
						: undefined,
				codes:
					filters.codes.value.length > 0
						? filters.codes.value.join(",")
						: undefined,
				source: filters.search.value || undefined,
			},
		});
		const data = response.data as {
			items?: DispatchJob[];
			totalItems?: number;
		};
		if (data) {
			dispatchJobs.value = (data.items || []) as DispatchJob[];
			totalRecords.value = data.totalItems || 0;
		}
	} catch (error) {
		console.error("Failed to load dispatch jobs:", error);
	} finally {
		loading.value = false;
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

		page.value = 0;
		await loadFilterOptions();
		await loadDispatchJobs();
	} finally {
		isUpdating.value = false;
	}
}

async function onStatusChange() {
	page.value = 0;
	await loadDispatchJobs();
}

async function onSearchChange() {
	page.value = 0;
	await loadDispatchJobs();
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
	return `${job.attemptCount || 0}/${job.maxRetries || 3}`;
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
          <MultiSelect
            v-model="filters.clients.value"
            :options="clientOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Clients"
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
            @click="loadDispatchJobs"
            v-tooltip="'Refresh'"
          />
        </div>
      </div>

      <DataTable
        :value="dispatchJobs"
        :loading="loading"
        :lazy="true"
        :paginator="true"
        :rows="pageSize"
        :totalRecords="totalRecords"
        :rowsPerPageOptions="[50, 100, 250, 500]"
        @page="onPage"
        @sort="onSort"
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
        <Column field="source" header="Source" sortable />
        <Column field="status" header="Status" sortable style="width: 8rem">
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
        <Column field="createdAt" header="Created" sortable style="width: 10rem">
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
    </div>
  </div>
</template>

<style scoped>
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
