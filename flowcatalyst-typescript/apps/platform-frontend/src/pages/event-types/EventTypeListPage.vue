<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import Tag from 'primevue/tag';
import MultiSelect from 'primevue/multiselect';
import Select from 'primevue/select';
import ProgressSpinner from 'primevue/progressspinner';
import { useEventTypes } from '@/composables/useEventTypes';
import type { EventType, SpecVersion } from '@/api/event-types';

const router = useRouter();
const {
  eventTypes,
  initialLoading,
  loading,
  selectedApplications,
  selectedSubdomains,
  selectedAggregates,
  selectedStatus,
  hasActiveFilters,
  applicationOptions,
  subdomainOptions,
  aggregateOptions,
  statusOptions,
  clearFilters,
  initialize,
} = useEventTypes();

onMounted(() => initialize());

function viewEventType(eventType: EventType) {
  router.push(`/event-types/${eventType.id}`);
}

function getSchemaStatusSeverity(status: string) {
  switch (status) {
    case 'CURRENT': return 'success';
    case 'FINALISING': return 'info';
    case 'DEPRECATED': return 'warn';
    default: return 'secondary';
  }
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Event Types</h1>
        <p class="page-subtitle">Manage event type definitions and schemas</p>
      </div>
      <Button
        label="Create Event Type"
        icon="pi pi-plus"
        @click="router.push('/event-types/create')"
      />
    </header>

    <!-- Filters -->
    <div class="fc-card filter-card">
      <div class="filter-row">
        <div class="filter-group">
          <label>Applications</label>
          <MultiSelect
            v-model="selectedApplications"
            :options="applicationOptions"
            placeholder="All Applications"
            :showClear="true"
            :maxSelectedLabels="2"
            selectedItemsLabel="{0} apps selected"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Subdomains</label>
          <MultiSelect
            v-model="selectedSubdomains"
            :options="subdomainOptions"
            placeholder="All Subdomains"
            :showClear="true"
            :maxSelectedLabels="2"
            selectedItemsLabel="{0} subdomains selected"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Aggregates</label>
          <MultiSelect
            v-model="selectedAggregates"
            :options="aggregateOptions"
            placeholder="All Aggregates"
            :showClear="true"
            :maxSelectedLabels="2"
            selectedItemsLabel="{0} aggregates selected"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Status</label>
          <Select
            v-model="selectedStatus"
            :options="statusOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Statuses"
            :showClear="true"
            class="filter-input"
          />
        </div>

        <div class="filter-actions">
          <Button
            v-if="hasActiveFilters"
            label="Clear Filters"
            icon="pi pi-filter-slash"
            text
            severity="secondary"
            @click="clearFilters"
          />
        </div>
      </div>
    </div>

    <!-- Data Table -->
    <div class="fc-card table-card">
      <div v-if="initialLoading" class="loading-container">
        <ProgressSpinner strokeWidth="3" />
      </div>

      <DataTable
        v-else
        :value="eventTypes"
        :loading="loading"
        :paginator="true"
        :rows="15"
        :rowsPerPageOptions="[10, 15, 25, 50]"
        :showCurrentPageReport="true"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} event types"
        class="p-datatable-sm"
        @row-click="(e) => viewEventType(e.data)"
        :rowClass="() => 'clickable-row'"
      >
        <Column header="Code" style="width: 30%">
          <template #body="{ data }">
            <div class="code-display">
              <span class="code-segment app">{{ data.application }}</span>
              <span class="code-separator">:</span>
              <span class="code-segment subdomain">{{ data.subdomain }}</span>
              <span class="code-separator">:</span>
              <span class="code-segment aggregate">{{ data.aggregate }}</span>
              <span class="code-separator">:</span>
              <span class="code-segment event">{{ data.event }}</span>
            </div>
          </template>
        </Column>

        <Column field="name" header="Name" style="width: 20%">
          <template #body="{ data }">
            <span class="name-text">{{ data.name }}</span>
          </template>
        </Column>

        <Column field="description" header="Description" style="width: 25%">
          <template #body="{ data }">
            <span class="description-text" v-tooltip.top="data.description">
              {{ data.description || '—' }}
            </span>
          </template>
        </Column>

        <Column header="Schemas" style="width: 10%">
          <template #body="{ data }">
            <div class="schema-badges">
              <Tag
                v-for="sv in data.specVersions"
                :key="sv.version"
                :value="sv.version"
                :severity="getSchemaStatusSeverity(sv.status)"
                v-tooltip.top="sv.status"
              />
              <span v-if="data.specVersions.length === 0" class="no-schemas">
                No schemas
              </span>
            </div>
          </template>
        </Column>

        <Column header="Status" style="width: 10%">
          <template #body="{ data }">
            <Tag
              :value="data.status"
              :severity="data.status === 'CURRENT' ? 'success' : 'secondary'"
            />
          </template>
        </Column>

        <Column style="width: 5%">
          <template #body="{ data }">
            <Button
              icon="pi pi-chevron-right"
              rounded
              text
              severity="secondary"
              v-tooltip.left="'View details'"
              @click.stop="viewEventType(data)"
            />
          </template>
        </Column>

        <template #empty>
          <div class="empty-message">
            <i class="pi pi-inbox"></i>
            <span>No event types found</span>
            <Button
              v-if="hasActiveFilters"
              label="Clear filters"
              link
              @click="clearFilters"
            />
          </div>
        </template>
      </DataTable>
    </div>
  </div>
</template>

<style scoped>
.filter-card {
  margin-bottom: 24px;
}

.filter-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: flex-end;
}

.filter-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 180px;
}

.filter-group label {
  font-size: 13px;
  font-weight: 500;
  color: #475569;
}

.filter-input {
  width: 100%;
}

.filter-actions {
  margin-left: auto;
}

.table-card {
  padding: 0;
  overflow: hidden;
}

.loading-container {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 60px;
}

.name-text {
  font-weight: 500;
  color: #1e293b;
}

.description-text {
  color: #64748b;
  font-size: 13px;
  display: block;
  max-width: 250px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.schema-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.no-schemas {
  color: #94a3b8;
  font-size: 12px;
  font-style: italic;
}

.empty-message {
  text-align: center;
  padding: 48px 24px;
  color: #64748b;
}

.empty-message i {
  font-size: 48px;
  display: block;
  margin-bottom: 16px;
  color: #cbd5e1;
}

.empty-message span {
  display: block;
  margin-bottom: 12px;
}

:deep(.clickable-row) {
  cursor: pointer;
  transition: background-color 0.15s;
}

:deep(.clickable-row:hover) {
  background-color: #f1f5f9 !important;
}

:deep(.p-datatable .p-datatable-thead > tr > th) {
  background: #f8fafc;
  color: #475569;
  font-weight: 600;
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.025em;
}

@media (max-width: 1024px) {
  .filter-row {
    flex-direction: column;
    align-items: stretch;
  }

  .filter-group {
    min-width: 100%;
  }

  .filter-actions {
    margin-left: 0;
    margin-top: 8px;
  }
}
</style>
