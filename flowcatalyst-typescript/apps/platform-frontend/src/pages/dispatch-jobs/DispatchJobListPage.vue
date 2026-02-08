<script setup lang="ts">
import { ref, onMounted } from 'vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Tag from 'primevue/tag';
import MultiSelect from 'primevue/multiselect';
import { getApiBffDispatchJobs, getApiBffDispatchJobsFilterOptions } from '@/api/generated';

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
const searchQuery = ref('');
const totalRecords = ref(0);
const currentPage = ref(0);
const pageSize = ref(20);

// Cascading filter selections
const selectedClients = ref<string[]>([]);
const selectedApplications = ref<string[]>([]);
const selectedSubdomains = ref<string[]>([]);
const selectedAggregates = ref<string[]>([]);
const selectedCodes = ref<string[]>([]);
const selectedStatuses = ref<string[]>([]);

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
    const response = await getApiBffDispatchJobsFilterOptions({
      query: {
        clientIds: selectedClients.value.length > 0 ? selectedClients.value.join(',') : undefined,
        applications: selectedApplications.value.length > 0 ? selectedApplications.value.join(',') : undefined,
        subdomains: selectedSubdomains.value.length > 0 ? selectedSubdomains.value.join(',') : undefined,
        aggregates: selectedAggregates.value.length > 0 ? selectedAggregates.value.join(',') : undefined
      }
    });
    const data = response.data as { clients?: FilterOption[]; applications?: FilterOption[]; subdomains?: FilterOption[]; aggregates?: FilterOption[]; codes?: FilterOption[]; statuses?: FilterOption[] };
    if (data) {
      clientOptions.value = (data.clients || []) as FilterOption[];
      applicationOptions.value = (data.applications || []) as FilterOption[];
      subdomainOptions.value = (data.subdomains || []) as FilterOption[];
      aggregateOptions.value = (data.aggregates || []) as FilterOption[];
      codeOptions.value = (data.codes || []) as FilterOption[];
      statusOptions.value = (data.statuses || []) as FilterOption[];
    }
  } catch (error) {
    console.error('Failed to load filter options:', error);
  }
}

async function loadDispatchJobs() {
  loading.value = true;
  try {
    const response = await getApiBffDispatchJobs({
      query: {
        page: currentPage.value,
        size: pageSize.value,
        clientIds: selectedClients.value.length > 0 ? selectedClients.value.join(',') : undefined,
        statuses: selectedStatuses.value.length > 0 ? selectedStatuses.value.join(',') : undefined,
        applications: selectedApplications.value.length > 0 ? selectedApplications.value.join(',') : undefined,
        subdomains: selectedSubdomains.value.length > 0 ? selectedSubdomains.value.join(',') : undefined,
        aggregates: selectedAggregates.value.length > 0 ? selectedAggregates.value.join(',') : undefined,
        codes: selectedCodes.value.length > 0 ? selectedCodes.value.join(',') : undefined,
        source: searchQuery.value || undefined
      }
    });
    const data = response.data as { items?: DispatchJob[]; totalItems?: number };
    if (data) {
      dispatchJobs.value = (data.items || []) as DispatchJob[];
      totalRecords.value = data.totalItems || 0;
    }
  } catch (error) {
    console.error('Failed to load dispatch jobs:', error);
  } finally {
    loading.value = false;
  }
}

async function onPage(event: { page: number; rows: number }) {
  currentPage.value = event.page;
  pageSize.value = event.rows;
  await loadDispatchJobs();
}

async function onFilterChange(clearDownstream: 'applications' | 'subdomains' | 'aggregates' | 'codes' | 'none' = 'none') {
  if (isUpdating.value) return;
  isUpdating.value = true;
  try {
    // Clear downstream selections based on which filter changed
    if (clearDownstream === 'applications') {
      selectedApplications.value = [];
      selectedSubdomains.value = [];
      selectedAggregates.value = [];
      selectedCodes.value = [];
    } else if (clearDownstream === 'subdomains') {
      selectedSubdomains.value = [];
      selectedAggregates.value = [];
      selectedCodes.value = [];
    } else if (clearDownstream === 'aggregates') {
      selectedAggregates.value = [];
      selectedCodes.value = [];
    } else if (clearDownstream === 'codes') {
      selectedCodes.value = [];
    }

    currentPage.value = 0;
    await loadFilterOptions();
    await loadDispatchJobs();
  } finally {
    isUpdating.value = false;
  }
}

async function onStatusChange() {
  currentPage.value = 0;
  await loadDispatchJobs();
}

async function onSearchChange() {
  currentPage.value = 0;
  await loadDispatchJobs();
}

function getSeverity(status: string): "success" | "info" | "warn" | "danger" | "secondary" | "contrast" | undefined {
  switch (status) {
    case 'COMPLETED': return 'success';
    case 'PENDING': return 'info';
    case 'QUEUED': return 'info';
    case 'IN_PROGRESS': return 'warn';
    case 'ERROR': return 'danger';
    case 'CANCELLED': return 'secondary';
    default: return 'secondary';
  }
}

function getModeSeverity(mode: string): "success" | "info" | "warn" | "danger" | "secondary" | "contrast" | undefined {
  switch (mode) {
    case 'IMMEDIATE': return 'success';
    case 'NEXT_ON_ERROR': return 'warn';
    case 'BLOCK_ON_ERROR': return 'danger';
    default: return 'secondary';
  }
}

function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleString();
}

function formatAttempts(job: DispatchJob): string {
  return `${job.attemptCount || 0}/${job.maxRetries || 3}`;
}

function formatCode(code: string | undefined): { app?: string; subdomain?: string; aggregate?: string; event?: string } {
  if (!code) return {};
  const parts = code.split(':');
  return {
    app: parts[0],
    subdomain: parts[1],
    aggregate: parts[2],
    event: parts[3]
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
            v-model="selectedClients"
            :options="clientOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Clients"
            class="filter-select"
            @change="onFilterChange('applications')"
          />
          <MultiSelect
            v-model="selectedApplications"
            :options="applicationOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Applications"
            class="filter-select"
            @change="onFilterChange('subdomains')"
          />
          <MultiSelect
            v-model="selectedSubdomains"
            :options="subdomainOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Subdomains"
            class="filter-select"
            @change="onFilterChange('aggregates')"
          />
          <MultiSelect
            v-model="selectedAggregates"
            :options="aggregateOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Aggregates"
            class="filter-select"
            @change="onFilterChange('codes')"
          />
          <MultiSelect
            v-model="selectedCodes"
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
            v-model="selectedStatuses"
            :options="statusOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Statuses"
            class="filter-select"
            @change="onStatusChange"
          />
          <span class="p-input-icon-left">
            <i class="pi pi-search" />
            <InputText
              v-model="searchQuery"
              placeholder="Search by source..."
              @keyup.enter="onSearchChange"
            />
          </span>
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
        :rowsPerPageOptions="[10, 20, 50]"
        @page="onPage"
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
            <span class="text-sm truncate" style="max-width: 200px; display: inline-block;">
              {{ data.targetUrl }}
            </span>
          </template>
        </Column>
        <Column field="createdAt" header="Created" sortable style="width: 10rem">
          <template #body="{ data }">
            <span class="text-sm">{{ formatDate(data.createdAt) }}</span>
          </template>
        </Column>
        <Column header="Actions" style="width: 6rem">
          <template #body="{ data }">
            <Button icon="pi pi-eye" text rounded v-tooltip="'View details'" />
            <Button
              icon="pi pi-replay"
              text
              rounded
              v-tooltip="'Retry'"
              :disabled="data.status === 'COMPLETED' || data.status === 'IN_PROGRESS'"
            />
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
</style>
