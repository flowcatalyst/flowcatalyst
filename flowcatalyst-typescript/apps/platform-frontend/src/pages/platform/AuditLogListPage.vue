<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Select from 'primevue/select';
import Button from 'primevue/button';
import Tag from 'primevue/tag';
import Dialog from 'primevue/dialog';
import ProgressSpinner from 'primevue/progressspinner';
import {
  fetchAuditLogs,
  fetchAuditLogById,
  fetchEntityTypes,
  fetchOperations,
  type AuditLog,
  type AuditLogDetail,
} from '@/api/audit-logs';

const auditLogs = ref<AuditLog[]>([]);
const totalRecords = ref(0);
const loading = ref(false);
const initialLoading = ref(true);

// Filters
const entityTypes = ref<string[]>([]);
const operations = ref<string[]>([]);
const selectedEntityType = ref<string | null>(null);
const selectedOperation = ref<string | null>(null);

// Pagination
const page = ref(0);
const pageSize = ref(25);

// Detail dialog
const selectedLog = ref<AuditLogDetail | null>(null);
const showDetailDialog = ref(false);
const loadingDetail = ref(false);

const hasActiveFilters = computed(() => {
  return selectedEntityType.value !== null || selectedOperation.value !== null;
});

async function loadFilters() {
  try {
    const [entityTypesRes, operationsRes] = await Promise.all([
      fetchEntityTypes(),
      fetchOperations(),
    ]);
    entityTypes.value = entityTypesRes.entityTypes;
    operations.value = operationsRes.operations;
  } catch (error) {
    console.error('Failed to load filters:', error);
  }
}

async function loadAuditLogs() {
  loading.value = true;
  try {
    const response = await fetchAuditLogs({
      entityType: selectedEntityType.value || undefined,
      operation: selectedOperation.value || undefined,
      page: page.value,
      pageSize: pageSize.value,
    });
    auditLogs.value = response.auditLogs;
    totalRecords.value = response.total;
  } catch (error) {
    console.error('Failed to load audit logs:', error);
  } finally {
    loading.value = false;
    initialLoading.value = false;
  }
}

async function viewDetails(log: AuditLog) {
  loadingDetail.value = true;
  showDetailDialog.value = true;
  try {
    selectedLog.value = await fetchAuditLogById(log.id);
  } catch (error) {
    console.error('Failed to load audit log details:', error);
  } finally {
    loadingDetail.value = false;
  }
}

function onPage(event: { page: number; rows: number }) {
  page.value = event.page;
  pageSize.value = event.rows;
  loadAuditLogs();
}

function clearFilters() {
  selectedEntityType.value = null;
  selectedOperation.value = null;
  page.value = 0;
  loadAuditLogs();
}

function applyFilters() {
  page.value = 0;
  loadAuditLogs();
}

function formatDateTime(isoString: string): string {
  return new Date(isoString).toLocaleString();
}

function formatOperationName(operation: string): string {
  // Convert camelCase/PascalCase to readable format
  return operation
    .replace(/([A-Z])/g, ' $1')
    .replace(/^./, str => str.toUpperCase())
    .trim();
}

function getEntityTypeSeverity(entityType: string): string {
  const types: Record<string, string> = {
    'ClientAuthConfig': 'info',
    'Role': 'warn',
    'Principal': 'success',
    'Application': 'secondary',
    'Client': 'secondary',
    'EventType': 'info',
  };
  return types[entityType] || 'secondary';
}

function formatJson(json: string | null): string {
  if (!json) return 'No data';
  try {
    return JSON.stringify(JSON.parse(json), null, 2);
  } catch {
    return json;
  }
}

onMounted(async () => {
  await loadFilters();
  await loadAuditLogs();
});
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Audit Log</h1>
        <p class="page-subtitle">View system activity and changes</p>
      </div>
    </header>

    <!-- Filters -->
    <div class="fc-card filter-card">
      <div class="filter-row">
        <div class="filter-group">
          <label>Entity Type</label>
          <Select
            v-model="selectedEntityType"
            :options="entityTypes"
            placeholder="All Entity Types"
            :showClear="true"
            class="filter-input"
            @change="applyFilters"
          />
        </div>

        <div class="filter-group">
          <label>Operation</label>
          <Select
            v-model="selectedOperation"
            :options="operations"
            placeholder="All Operations"
            :showClear="true"
            class="filter-input"
            @change="applyFilters"
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
        :value="auditLogs"
        :loading="loading"
        :paginator="true"
        :rows="pageSize"
        :totalRecords="totalRecords"
        :rowsPerPageOptions="[10, 25, 50, 100]"
        :lazy="true"
        :showCurrentPageReport="true"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} entries"
        class="p-datatable-sm"
        @page="onPage"
        @row-click="(e) => viewDetails(e.data)"
        :rowClass="() => 'clickable-row'"
      >
        <Column field="performedAt" header="Time" style="width: 15%">
          <template #body="{ data }">
            <span class="time-text">{{ formatDateTime(data.performedAt) }}</span>
          </template>
        </Column>

        <Column field="entityType" header="Entity Type" style="width: 15%">
          <template #body="{ data }">
            <Tag
              :value="data.entityType"
              :severity="getEntityTypeSeverity(data.entityType)"
            />
          </template>
        </Column>

        <Column field="entityId" header="Entity ID" style="width: 15%">
          <template #body="{ data }">
            <code class="entity-id">{{ data.entityId }}</code>
          </template>
        </Column>

        <Column field="operation" header="Operation" style="width: 20%">
          <template #body="{ data }">
            <span class="operation-text">{{ formatOperationName(data.operation) }}</span>
          </template>
        </Column>

        <Column field="principalName" header="Performed By" style="width: 20%">
          <template #body="{ data }">
            <span class="principal-text">{{ data.principalName || 'Unknown' }}</span>
          </template>
        </Column>

        <Column style="width: 5%">
          <template #body="{ data }">
            <Button
              icon="pi pi-eye"
              rounded
              text
              severity="secondary"
              v-tooltip.left="'View details'"
              @click.stop="viewDetails(data)"
            />
          </template>
        </Column>

        <template #empty>
          <div class="empty-message">
            <i class="pi pi-inbox"></i>
            <span>No audit logs found</span>
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

    <!-- Detail Dialog -->
    <Dialog
      v-model:visible="showDetailDialog"
      header="Audit Log Details"
      :modal="true"
      :style="{ width: '700px' }"
      :closable="true"
    >
      <div v-if="loadingDetail" class="dialog-loading">
        <ProgressSpinner strokeWidth="3" />
      </div>

      <div v-else-if="selectedLog" class="detail-content">
        <div class="detail-grid">
          <div class="detail-row">
            <span class="detail-label">Time</span>
            <span class="detail-value">{{ formatDateTime(selectedLog.performedAt) }}</span>
          </div>

          <div class="detail-row">
            <span class="detail-label">Entity Type</span>
            <Tag :value="selectedLog.entityType" :severity="getEntityTypeSeverity(selectedLog.entityType)" />
          </div>

          <div class="detail-row">
            <span class="detail-label">Entity ID</span>
            <code class="entity-id">{{ selectedLog.entityId }}</code>
          </div>

          <div class="detail-row">
            <span class="detail-label">Operation</span>
            <span class="detail-value">{{ formatOperationName(selectedLog.operation) }}</span>
          </div>

          <div class="detail-row">
            <span class="detail-label">Performed By</span>
            <span class="detail-value">{{ selectedLog.principalName || 'Unknown' }}</span>
          </div>

          <div class="detail-row" v-if="selectedLog.principalId">
            <span class="detail-label">Principal ID</span>
            <code class="entity-id">{{ selectedLog.principalId }}</code>
          </div>
        </div>

        <div class="operation-data" v-if="selectedLog.operationJson">
          <h4>Operation Data</h4>
          <pre class="json-display">{{ formatJson(selectedLog.operationJson) }}</pre>
        </div>
      </div>
    </Dialog>
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
  min-width: 200px;
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

.time-text {
  font-size: 13px;
  color: #64748b;
}

.entity-id {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  background: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
  color: #475569;
}

.operation-text {
  font-weight: 500;
  color: #1e293b;
}

.principal-text {
  color: #475569;
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

/* Dialog styles */
.dialog-loading {
  display: flex;
  justify-content: center;
  padding: 40px;
}

.detail-content {
  padding: 8px 0;
}

.detail-grid {
  display: grid;
  gap: 16px;
}

.detail-row {
  display: flex;
  align-items: center;
  gap: 16px;
}

.detail-label {
  min-width: 120px;
  font-size: 13px;
  font-weight: 500;
  color: #64748b;
}

.detail-value {
  color: #1e293b;
}

.operation-data {
  margin-top: 24px;
  border-top: 1px solid #e2e8f0;
  padding-top: 16px;
}

.operation-data h4 {
  margin: 0 0 12px 0;
  font-size: 14px;
  font-weight: 600;
  color: #475569;
}

.json-display {
  background: #1e293b;
  color: #e2e8f0;
  padding: 16px;
  border-radius: 8px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  overflow-x: auto;
  max-height: 300px;
  margin: 0;
}

@media (max-width: 768px) {
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
