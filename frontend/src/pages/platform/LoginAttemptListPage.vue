<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useListState } from "@/composables/useListState";
import {
  fetchLoginAttempts,
  type LoginAttempt,
} from "@/api/login-attempts";

const attempts = ref<LoginAttempt[]>([]);
const totalRecords = ref(0);
const loading = ref(false);
const initialLoading = ref(true);

const { filters, page, pageSize, sortField, sortOrder, hasActiveFilters, clearFilters, onPage, onSort } =
  useListState(
    {
      filters: {
        attemptType: { type: "string", key: "type" },
        outcome: { type: "string", key: "outcome" },
        identifier: { type: "string", key: "id" },
        dateFrom: { type: "string", key: "from" },
        dateTo: { type: "string", key: "to" },
      },
      pageSize: 100,
      sortField: "attemptedAt",
      sortOrder: "desc",
      debounce: 0,
    },
    () => loadAttempts(),
  );

// Detail dialog
const selectedAttempt = ref<LoginAttempt | null>(null);
const showDetailDialog = ref(false);

const attemptTypeOptions = ["USER_LOGIN", "SERVICE_ACCOUNT_TOKEN"];
const outcomeOptions = ["SUCCESS", "FAILURE"];

async function loadAttempts() {
  loading.value = true;
  try {
    const response = await fetchLoginAttempts({
      attemptType: filters.attemptType.value || undefined,
      outcome: filters.outcome.value || undefined,
      identifier: filters.identifier.value.trim() || undefined,
      dateFrom: filters.dateFrom.value || undefined,
      dateTo: filters.dateTo.value || undefined,
      page: page.value,
      pageSize: pageSize.value,
      sortField: sortField.value,
      sortOrder: sortOrder.value,
    });
    attempts.value = response.items;
    totalRecords.value = response.total;
  } catch (error) {
    console.error("Failed to load login attempts:", error);
  } finally {
    loading.value = false;
    initialLoading.value = false;
  }
}

function viewDetails(attempt: LoginAttempt) {
  selectedAttempt.value = attempt;
  showDetailDialog.value = true;
}

function formatDateTime(isoString: string): string {
  return new Date(isoString).toLocaleString();
}

function formatAttemptType(type: string): string {
  return type === "USER_LOGIN" ? "User Login" : "Service Account";
}

function formatFailureReason(reason: string | null): string {
  if (!reason) return "";
  return reason
    .replace(/_/g, " ")
    .toLowerCase()
    .replace(/^./, (c) => c.toUpperCase());
}

function outcomeSeverity(outcome: string): string {
  return outcome === "SUCCESS" ? "success" : "danger";
}

function attemptTypeSeverity(type: string): string {
  return type === "USER_LOGIN" ? "info" : "secondary";
}

onMounted(async () => {
  await loadAttempts();
});
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Login Attempts</h1>
        <p class="page-subtitle">Authentication attempt history for users and service accounts</p>
      </div>
    </header>

    <!-- Filters -->
    <div class="fc-card filter-card">
      <div class="filter-row">
        <div class="filter-group">
          <label>Attempt Type</label>
          <Select
            v-model="filters.attemptType.value"
            :options="attemptTypeOptions"
            placeholder="All Types"
            :showClear="true"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Outcome</label>
          <Select
            v-model="filters.outcome.value"
            :options="outcomeOptions"
            placeholder="All Outcomes"
            :showClear="true"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Identifier</label>
          <InputText
            v-model="filters.identifier.value"
            placeholder="Email or client_id"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>From</label>
          <InputText
            v-model="filters.dateFrom.value"
            type="datetime-local"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>To</label>
          <InputText
            v-model="filters.dateTo.value"
            type="datetime-local"
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
        :value="attempts"
        :loading="loading"
        :paginator="true"
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
        <Column field="attemptedAt" header="Time" sortable style="width: 16%">
          <template #body="{ data }">
            <span class="time-text">{{ formatDateTime(data.attemptedAt) }}</span>
          </template>
        </Column>

        <Column field="attemptType" header="Type" sortable style="width: 14%">
          <template #body="{ data }">
            <Tag
              :value="formatAttemptType(data.attemptType)"
              :severity="attemptTypeSeverity(data.attemptType)"
            />
          </template>
        </Column>

        <Column field="outcome" header="Outcome" sortable style="width: 10%">
          <template #body="{ data }">
            <Tag :value="data.outcome" :severity="outcomeSeverity(data.outcome)" />
          </template>
        </Column>

        <Column field="identifier" header="Identifier" sortable style="width: 22%">
          <template #body="{ data }">
            <code class="identifier-text">{{ data.identifier }}</code>
          </template>
        </Column>

        <Column field="failureReason" header="Failure Reason" style="width: 18%">
          <template #body="{ data }">
            <span v-if="data.failureReason" class="failure-reason">
              {{ formatFailureReason(data.failureReason) }}
            </span>
            <span v-else class="muted-text">—</span>
          </template>
        </Column>

        <Column field="ipAddress" header="IP Address" style="width: 14%">
          <template #body="{ data }">
            <code v-if="data.ipAddress" class="ip-text">{{ data.ipAddress }}</code>
            <span v-else class="muted-text">—</span>
          </template>
        </Column>

        <Column style="width: 6%">
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
            <span>No login attempts found</span>
            <Button v-if="hasActiveFilters" label="Clear filters" link @click="clearFilters" />
          </div>
        </template>
      </DataTable>
    </div>

    <!-- Detail Dialog -->
    <Dialog
      v-model:visible="showDetailDialog"
      header="Login Attempt Details"
      :modal="true"
      :style="{ width: '600px' }"
      :closable="true"
    >
      <div v-if="selectedAttempt" class="detail-content">
        <div class="detail-grid">
          <div class="detail-row">
            <span class="detail-label">Time</span>
            <span class="detail-value">{{ formatDateTime(selectedAttempt.attemptedAt) }}</span>
          </div>

          <div class="detail-row">
            <span class="detail-label">Type</span>
            <Tag
              :value="formatAttemptType(selectedAttempt.attemptType)"
              :severity="attemptTypeSeverity(selectedAttempt.attemptType)"
            />
          </div>

          <div class="detail-row">
            <span class="detail-label">Outcome</span>
            <Tag
              :value="selectedAttempt.outcome"
              :severity="outcomeSeverity(selectedAttempt.outcome)"
            />
          </div>

          <div class="detail-row" v-if="selectedAttempt.failureReason">
            <span class="detail-label">Failure Reason</span>
            <span class="failure-reason detail-value">
              {{ formatFailureReason(selectedAttempt.failureReason) }}
            </span>
          </div>

          <div class="detail-row">
            <span class="detail-label">Identifier</span>
            <code class="identifier-text">{{ selectedAttempt.identifier }}</code>
          </div>

          <div class="detail-row" v-if="selectedAttempt.principalId">
            <span class="detail-label">Principal ID</span>
            <code class="identifier-text">{{ selectedAttempt.principalId }}</code>
          </div>

          <div class="detail-row" v-if="selectedAttempt.ipAddress">
            <span class="detail-label">IP Address</span>
            <code class="ip-text">{{ selectedAttempt.ipAddress }}</code>
          </div>

          <div class="detail-row" v-if="selectedAttempt.userAgent">
            <span class="detail-label">User Agent</span>
            <span class="detail-value user-agent-text">{{ selectedAttempt.userAgent }}</span>
          </div>
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

.time-text {
  font-size: 13px;
  color: #64748b;
}

.identifier-text {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  background: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
  color: #475569;
}

.ip-text {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  color: #64748b;
}

.failure-reason {
  font-size: 13px;
  color: #dc2626;
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
.detail-content {
  padding: 8px 0;
}

.detail-grid {
  display: grid;
  gap: 16px;
}

.detail-row {
  display: flex;
  align-items: flex-start;
  gap: 16px;
}

.detail-label {
  min-width: 120px;
  font-size: 13px;
  font-weight: 500;
  color: #64748b;
  padding-top: 2px;
}

.detail-value {
  color: #1e293b;
}

.user-agent-text {
  font-size: 12px;
  color: #64748b;
  word-break: break-all;
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
