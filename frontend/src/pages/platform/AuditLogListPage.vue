<script setup lang="ts">
import { ref, onMounted } from "vue";
import {
	fetchAuditLogs,
	fetchAuditLogById,
	fetchEntityTypes,
	fetchOperations,
	fetchDistinctApplicationIds,
	type AuditLog,
	type AuditLogDetail,
} from "@/api/audit-logs";
import { applicationsApi } from "@/api/applications";
import { useListState } from "@/composables/useListState";
import { useClientOptions } from "@/composables/useClientOptions";
import ClientFilter from "@/components/ClientFilter.vue";

interface Option {
	label: string;
	value: string;
}

const auditLogs = ref<AuditLog[]>([]);
const totalRecords = ref(0);
const loading = ref(false);
const initialLoading = ref(true);

// Filter option lists
const entityTypes = ref<string[]>([]);
const operations = ref<string[]>([]);
const applicationOptions = ref<Option[]>([]);
const { ensureLoaded: ensureClients, getLabel: getClientLabel } = useClientOptions();

const { filters, page, pageSize, sortField, sortOrder, hasActiveFilters, clearFilters: doClearFilters, onPage, onSort } =
	useListState(
		{
			filters: {
				entityType:      { type: "string", key: "entityType" },
				operation:       { type: "string", key: "operation" },
				applicationIds:  { type: "array",  key: "appIds" },
				clientIds:       { type: "array",  key: "clientIds" },
			},
			pageSize: 100,
			sortField: "performedAt",
			sortOrder: "desc",
			debounce: 0,
		},
		() => loadAuditLogs(),
	);

// Detail dialog
const selectedLog = ref<AuditLogDetail | null>(null);
const showDetailDialog = ref(false);
const loadingDetail = ref(false);

async function loadFilters() {
	try {
		const [entityTypesRes, operationsRes, appIdsRes, appsRes] =
			await Promise.all([
				fetchEntityTypes(),
				fetchOperations(),
				fetchDistinctApplicationIds(),
				applicationsApi.list(),
				ensureClients(),
			]);
		entityTypes.value = entityTypesRes.entityTypes;
		operations.value = operationsRes.operations;

		// Only show applications that actually appear in audit logs
		const appMap = new Map(appsRes.applications.map((a) => [a.id, a.name]));
		applicationOptions.value = appIdsRes.applicationIds.map((id) => ({
			label: appMap.get(id) ?? id,
			value: id,
		}));
	} catch (error) {
		console.error("Failed to load filters:", error);
	}
}

async function loadAuditLogs() {
	loading.value = true;
	try {
		const response = await fetchAuditLogs({
			entityType: filters.entityType.value || undefined,
			operation: filters.operation.value || undefined,
			applicationIds: filters.applicationIds.value.length
				? filters.applicationIds.value
				: undefined,
			clientIds: filters.clientIds.value.length
				? filters.clientIds.value
				: undefined,
			page: page.value,
			pageSize: pageSize.value,
			sortField: sortField.value,
			sortOrder: sortOrder.value,
		});
		auditLogs.value = response.auditLogs;
		totalRecords.value = response.total;
	} catch (error) {
		console.error("Failed to load audit logs:", error);
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
		console.error("Failed to load audit log details:", error);
	} finally {
		loadingDetail.value = false;
	}
}

function clearFilters() {
	doClearFilters();
}

function formatDateTime(isoString: string): string {
	return new Date(isoString).toLocaleString();
}

function formatOperationName(operation: string): string {
	return operation
		.replace(/([A-Z])/g, " $1")
		.replace(/^./, (str) => str.toUpperCase())
		.trim();
}

function getEntityTypeSeverity(entityType: string): string {
	const types: Record<string, string> = {
		ClientAuthConfig: "info",
		Role: "warn",
		Principal: "success",
		Application: "secondary",
		Client: "secondary",
		EventType: "info",
	};
	return types[entityType] || "secondary";
}

function formatJson(json: string | null): string {
	if (!json) return "No data";
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
            v-model="filters.entityType.value"
            :options="entityTypes"
            placeholder="All Entity Types"
            :showClear="true"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Operation</label>
          <Select
            v-model="filters.operation.value"
            :options="operations"
            placeholder="All Operations"
            :showClear="true"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Application</label>
          <MultiSelect
            v-model="filters.applicationIds.value"
            :options="applicationOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Applications"
            :maxSelectedLabels="2"
            filter
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Client</label>
          <ClientFilter
            v-model="filters.clientIds.value"
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
        :value="auditLogs"
        :loading="loading"
        :paginator="true"
        :first="page * pageSize"
        :rows="pageSize"
        :totalRecords="totalRecords"
        :rowsPerPageOptions="[50, 100, 250, 500]"
        :lazy="true"
        :showCurrentPageReport="true"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} entries"
        size="small"
        @page="onPage"
        @sort="onSort"
        @row-click="(e) => viewDetails(e.data)"
        :rowClass="() => 'clickable-row'"
      >
        <Column field="performedAt" header="Time" sortable style="width: 15%">
          <template #body="{ data }">
            <span class="time-text">{{ formatDateTime(data.performedAt) }}</span>
          </template>
        </Column>

        <Column field="entityType" header="Entity Type" sortable style="width: 13%">
          <template #body="{ data }">
            <Tag :value="data.entityType" :severity="getEntityTypeSeverity(data.entityType)" />
          </template>
        </Column>

        <Column field="entityId" header="Entity ID" style="width: 14%">
          <template #body="{ data }">
            <code class="entity-id">{{ data.entityId }}</code>
          </template>
        </Column>

        <Column field="operation" header="Operation" sortable style="width: 18%">
          <template #body="{ data }">
            <span class="operation-text">{{ formatOperationName(data.operation) }}</span>
          </template>
        </Column>

        <Column field="principalName" header="Performed By" style="width: 15%">
          <template #body="{ data }">
            <span class="principal-text">{{ data.principalName || 'Unknown' }}</span>
          </template>
        </Column>

        <Column header="Application" style="width: 13%">
          <template #body="{ data }">
            <span v-if="data.applicationId" class="context-tag">
              {{ applicationOptions.find(a => a.value === data.applicationId)?.label ?? data.applicationId }}
            </span>
            <span v-else class="muted-text">—</span>
          </template>
        </Column>

        <Column header="Client" style="width: 13%">
          <template #body="{ data }">
            <span v-if="data.clientId" class="context-tag">
              {{ getClientLabel(data.clientId) }}
            </span>
            <span v-else class="muted-text">—</span>
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
            <Button v-if="hasActiveFilters" label="Clear filters" link @click="clearFilters" />
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
            <Tag
              :value="selectedLog.entityType"
              :severity="getEntityTypeSeverity(selectedLog.entityType)"
            />
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

          <div class="detail-row" v-if="selectedLog.applicationId">
            <span class="detail-label">Application</span>
            <span class="detail-value">
              {{ applicationOptions.find(a => a.value === selectedLog!.applicationId)?.label ?? selectedLog.applicationId }}
            </span>
          </div>

          <div class="detail-row" v-if="selectedLog.clientId">
            <span class="detail-label">Client</span>
            <span class="detail-value">
              {{ getClientLabel(selectedLog.clientId) }}
            </span>
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

.context-tag {
  font-size: 13px;
  color: #334e68;
}

.muted-text {
  color: #94a3b8;
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
