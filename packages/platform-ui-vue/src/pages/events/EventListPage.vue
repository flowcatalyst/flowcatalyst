<script setup lang="ts">
import { ref, onMounted } from 'vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Tag from 'primevue/tag';
import Dialog from 'primevue/dialog';
import MultiSelect from 'primevue/multiselect';
import { getApiBffEvents, getApiBffEventsById, getApiBffEventsFilterOptions } from '@/api/generated';

interface EventRead {
  id: string;
  specVersion: string;
  type: string;
  application?: string;
  subdomain?: string;
  aggregate?: string;
  source: string;
  subject: string;
  time: string;
  data: string;
  messageGroup?: string;
  correlationId?: string;
  causationId?: string;
  deduplicationId?: string;
  contextData?: { key: string; value: string }[];
  clientId?: string;
  projectedAt?: string;
}

interface FilterOption {
  value: string;
  label: string;
}

// Table state
const events = ref<EventRead[]>([]);
const loading = ref(true);
const totalRecords = ref(0);
const currentPage = ref(0);
const pageSize = ref(20);

// Filter state
const selectedClients = ref<string[]>([]);
const selectedApplications = ref<string[]>([]);
const selectedSubdomains = ref<string[]>([]);
const selectedAggregates = ref<string[]>([]);
const selectedTypes = ref<string[]>([]);
const searchQuery = ref('');

// Filter options (from server)
const clientOptions = ref<FilterOption[]>([]);
const applicationOptions = ref<FilterOption[]>([]);
const subdomainOptions = ref<FilterOption[]>([]);
const aggregateOptions = ref<FilterOption[]>([]);
const typeOptions = ref<FilterOption[]>([]);
const loadingOptions = ref(false);

// Prevent infinite loops from cascading watchers
const isUpdating = ref(false);

// Detail dialog
const selectedEvent = ref<EventRead | null>(null);
const showDetailDialog = ref(false);
const loadingDetail = ref(false);

onMounted(async () => {
  await loadFilterOptions();
  await loadEvents();
});

// Unified filter change handler to prevent loops
async function onFilterChange(clearDownstream: 'applications' | 'subdomains' | 'aggregates' | 'types' | 'none' = 'none') {
  if (isUpdating.value) return;

  isUpdating.value = true;
  try {
    // Clear downstream selections based on which filter changed
    if (clearDownstream === 'applications') {
      selectedApplications.value = [];
      selectedSubdomains.value = [];
      selectedAggregates.value = [];
      selectedTypes.value = [];
    } else if (clearDownstream === 'subdomains') {
      selectedSubdomains.value = [];
      selectedAggregates.value = [];
      selectedTypes.value = [];
    } else if (clearDownstream === 'aggregates') {
      selectedAggregates.value = [];
      selectedTypes.value = [];
    } else if (clearDownstream === 'types') {
      selectedTypes.value = [];
    }

    await loadFilterOptions();
    await loadEvents();
  } finally {
    isUpdating.value = false;
  }
}


async function loadFilterOptions() {
  loadingOptions.value = true;
  try {
    const response = await getApiBffEventsFilterOptions({
      query: {
        clientIds: selectedClients.value.length ? selectedClients.value.join(',') : undefined,
        applications: selectedApplications.value.length ? selectedApplications.value.join(',') : undefined,
        subdomains: selectedSubdomains.value.length ? selectedSubdomains.value.join(',') : undefined,
        aggregates: selectedAggregates.value.length ? selectedAggregates.value.join(',') : undefined
      }
    });
    const data = response.data as { clients?: FilterOption[]; applications?: FilterOption[]; subdomains?: FilterOption[]; aggregates?: FilterOption[]; types?: FilterOption[] };
    if (data) {
      clientOptions.value = (data.clients || []) as FilterOption[];
      applicationOptions.value = (data.applications || []) as FilterOption[];
      subdomainOptions.value = (data.subdomains || []) as FilterOption[];
      aggregateOptions.value = (data.aggregates || []) as FilterOption[];
      typeOptions.value = (data.types || []) as FilterOption[];
    }
  } catch (error) {
    console.error('Failed to load filter options:', error);
  } finally {
    loadingOptions.value = false;
  }
}

async function loadEvents() {
  loading.value = true;
  try {
    const response = await getApiBffEvents({
      query: {
        page: currentPage.value,
        size: pageSize.value,
        clientIds: selectedClients.value.length ? selectedClients.value.join(',') : undefined,
        applications: selectedApplications.value.length ? selectedApplications.value.join(',') : undefined,
        subdomains: selectedSubdomains.value.length ? selectedSubdomains.value.join(',') : undefined,
        aggregates: selectedAggregates.value.length ? selectedAggregates.value.join(',') : undefined,
        types: selectedTypes.value.length ? selectedTypes.value.join(',') : undefined,
        source: searchQuery.value || undefined
      }
    });
    const data = response.data as { items?: EventRead[]; totalItems?: number };
    if (data) {
      events.value = (data.items || []) as EventRead[];
      totalRecords.value = data.totalItems || 0;
    }
  } catch (error) {
    console.error('Failed to load events:', error);
  } finally {
    loading.value = false;
  }
}

async function onPage(event: { page: number; rows: number }) {
  currentPage.value = event.page;
  pageSize.value = event.rows;
  await loadEvents();
}

async function onSearchChange() {
  currentPage.value = 0;
  await loadEvents();
}

async function clearAllFilters() {
  selectedClients.value = [];
  selectedApplications.value = [];
  selectedSubdomains.value = [];
  selectedAggregates.value = [];
  selectedTypes.value = [];
  searchQuery.value = '';
  await loadFilterOptions();
  await loadEvents();
}

async function viewEventDetail(event: EventRead) {
  loadingDetail.value = true;
  showDetailDialog.value = true;
  try {
    const response = await getApiBffEventsById({ path: { id: event.id } });
    if (response.data) {
      selectedEvent.value = response.data as EventRead;
    }
  } catch (error) {
    console.error('Failed to load event details:', error);
  } finally {
    loadingDetail.value = false;
  }
}

function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleString();
}

function formatData(data: string | undefined): string {
  if (!data) return '-';
  try {
    return JSON.stringify(JSON.parse(data), null, 2);
  } catch {
    return data;
  }
}

function truncateId(id: string | undefined): string {
  if (!id) return '-';
  return id.length > 10 ? `${id.slice(0, 10)}...` : id;
}

function hasActiveFilters(): boolean {
  return selectedClients.value.length > 0 ||
    selectedApplications.value.length > 0 ||
    selectedSubdomains.value.length > 0 ||
    selectedAggregates.value.length > 0 ||
    selectedTypes.value.length > 0 ||
    searchQuery.value.length > 0;
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Events</h1>
        <p class="page-subtitle">Browse events from the event store</p>
      </div>
    </header>

    <div class="fc-card">
      <!-- Cascading Filter Bar -->
      <div class="filter-bar">
        <div class="filter-row">
          <div class="filter-group">
            <label>Client</label>
            <MultiSelect
              v-model="selectedClients"
              :options="clientOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="All Clients"
              :maxSelectedLabels="2"
              :loading="loadingOptions"
              class="filter-select"
              filter
              @change="onFilterChange('applications')"
            />
          </div>
          <div class="filter-group">
            <label>Application</label>
            <MultiSelect
              v-model="selectedApplications"
              :options="applicationOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="All Applications"
              :maxSelectedLabels="2"
              :loading="loadingOptions"
              class="filter-select"
              filter
              @change="onFilterChange('subdomains')"
            />
          </div>
          <div class="filter-group">
            <label>Subdomain</label>
            <MultiSelect
              v-model="selectedSubdomains"
              :options="subdomainOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="All Subdomains"
              :maxSelectedLabels="2"
              :loading="loadingOptions"
              class="filter-select"
              filter
              @change="onFilterChange('aggregates')"
            />
          </div>
          <div class="filter-group">
            <label>Aggregate</label>
            <MultiSelect
              v-model="selectedAggregates"
              :options="aggregateOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="All Aggregates"
              :maxSelectedLabels="2"
              :loading="loadingOptions"
              class="filter-select"
              filter
              @change="onFilterChange('types')"
            />
          </div>
          <div class="filter-group">
            <label>Event Type</label>
            <MultiSelect
              v-model="selectedTypes"
              :options="typeOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="All Types"
              :maxSelectedLabels="1"
              :loading="loadingOptions"
              class="filter-select filter-select-wide"
              filter
              @change="onFilterChange('none')"
            />
          </div>
        </div>
        <div class="filter-actions">
          <span class="p-input-icon-left">
            <i class="pi pi-search" />
            <InputText
              v-model="searchQuery"
              placeholder="Search by source..."
              @keyup.enter="onSearchChange"
              class="search-input"
            />
          </span>
          <Button
            icon="pi pi-filter-slash"
            text
            rounded
            @click="clearAllFilters"
            v-tooltip="'Clear all filters'"
            :disabled="!hasActiveFilters()"
          />
          <Button
            icon="pi pi-refresh"
            text
            rounded
            @click="loadEvents"
            v-tooltip="'Refresh'"
          />
        </div>
      </div>

      <DataTable
        :value="events"
        :loading="loading"
        :lazy="true"
        :paginator="true"
        :rows="pageSize"
        :totalRecords="totalRecords"
        :rowsPerPageOptions="[10, 20, 50]"
        @page="onPage"
        stripedRows
        emptyMessage="No events found"
        tableStyle="min-width: 60rem"
      >
        <Column field="id" header="Event ID" style="width: 10rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ truncateId(data.id) }}</span>
          </template>
        </Column>
        <Column field="type" header="Type">
          <template #body="{ data }">
            <Tag :value="data.type" severity="info" />
          </template>
        </Column>
        <Column field="source" header="Source" />
        <Column field="subject" header="Subject">
          <template #body="{ data }">
            <span class="text-sm truncate-cell">{{ data.subject || '-' }}</span>
          </template>
        </Column>
        <Column field="clientId" header="Client" style="width: 10rem">
          <template #body="{ data }">
            <span v-if="data.clientId" class="font-mono text-sm">{{ truncateId(data.clientId) }}</span>
            <Tag v-else value="Platform" severity="secondary" />
          </template>
        </Column>
        <Column field="time" header="Time" style="width: 12rem">
          <template #body="{ data }">
            <span class="text-sm">{{ formatDate(data.time) }}</span>
          </template>
        </Column>
        <Column header="Actions" style="width: 6rem">
          <template #body="{ data }">
            <Button
              icon="pi pi-eye"
              text
              rounded
              v-tooltip="'View details'"
              @click="viewEventDetail(data)"
            />
          </template>
        </Column>
      </DataTable>
    </div>

    <!-- Event Detail Dialog -->
    <Dialog
      v-model:visible="showDetailDialog"
      header="Event Details"
      :style="{ width: '700px' }"
      modal
    >
      <div v-if="loadingDetail" class="flex justify-center p-4">
        <i class="pi pi-spin pi-spinner" style="font-size: 2rem"></i>
      </div>
      <div v-else-if="selectedEvent" class="event-detail">
        <div class="detail-row">
          <label>ID</label>
          <span class="font-mono">{{ selectedEvent.id }}</span>
        </div>
        <div class="detail-row">
          <label>Type</label>
          <Tag :value="selectedEvent.type" severity="info" />
        </div>
        <div class="detail-row">
          <label>Application</label>
          <span>{{ selectedEvent.application || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Subdomain</label>
          <span>{{ selectedEvent.subdomain || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Aggregate</label>
          <span>{{ selectedEvent.aggregate || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Source</label>
          <span>{{ selectedEvent.source }}</span>
        </div>
        <div class="detail-row">
          <label>Subject</label>
          <span>{{ selectedEvent.subject || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Time</label>
          <span>{{ formatDate(selectedEvent.time) }}</span>
        </div>
        <div class="detail-row">
          <label>Client ID</label>
          <span v-if="selectedEvent.clientId" class="font-mono">{{ selectedEvent.clientId }}</span>
          <Tag v-else value="Platform (No Client)" severity="secondary" />
        </div>
        <div class="detail-row">
          <label>Message Group</label>
          <span>{{ selectedEvent.messageGroup || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Correlation ID</label>
          <span class="font-mono">{{ selectedEvent.correlationId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Causation ID</label>
          <span class="font-mono">{{ selectedEvent.causationId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Deduplication ID</label>
          <span class="font-mono">{{ selectedEvent.deduplicationId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Projected At</label>
          <span>{{ formatDate(selectedEvent.projectedAt) }}</span>
        </div>
        <div class="detail-section">
          <label>Data</label>
          <pre class="data-block">{{ formatData(selectedEvent.data) }}</pre>
        </div>
        <div v-if="selectedEvent.contextData?.length" class="detail-section">
          <label>Context Data</label>
          <div class="context-data">
            <div v-for="cd in selectedEvent.contextData" :key="cd.key" class="context-item">
              <span class="context-key">{{ cd.key }}:</span>
              <span class="context-value">{{ cd.value }}</span>
            </div>
          </div>
        </div>
      </div>
    </Dialog>
  </div>
</template>

<style scoped>
.filter-bar {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-bottom: 1rem;
  padding: 0.75rem;
  background: var(--surface-ground);
  border-radius: 6px;
  border: 1px solid var(--surface-border);
}

.filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
}

.filter-group {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  min-width: 150px;
}

.filter-group label {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-color-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.filter-select {
  width: 180px;
}

.filter-select-wide {
  width: 280px;
}

.filter-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid var(--surface-border);
}

.search-input {
  width: 250px;
}

.font-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.text-sm {
  font-size: 0.875rem;
}

.truncate-cell {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
}

.event-detail {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.detail-row {
  display: flex;
  gap: 1rem;
}

.detail-row label {
  font-weight: 600;
  min-width: 120px;
  color: var(--text-color-secondary);
}

.detail-section {
  margin-top: 0.5rem;
}

.detail-section label {
  display: block;
  font-weight: 600;
  margin-bottom: 0.5rem;
  color: var(--text-color-secondary);
}

.data-block {
  background: var(--surface-ground);
  border: 1px solid var(--surface-border);
  border-radius: 6px;
  padding: 1rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.875rem;
  overflow-x: auto;
  max-height: 300px;
  white-space: pre-wrap;
  word-break: break-word;
}

.context-data {
  background: var(--surface-ground);
  border: 1px solid var(--surface-border);
  border-radius: 6px;
  padding: 0.75rem;
}

.context-item {
  padding: 0.25rem 0;
}

.context-key {
  font-weight: 500;
  margin-right: 0.5rem;
}

.context-value {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.flex {
  display: flex;
}

.justify-center {
  justify-content: center;
}

.p-4 {
  padding: 1rem;
}
</style>
