<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useToast } from 'primevue/usetoast';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import InputText from 'primevue/inputtext';
import Tag from 'primevue/tag';
import Select from 'primevue/select';
import Button from 'primevue/button';
import ProgressSpinner from 'primevue/progressspinner';
import { permissionsApi, type Permission } from '@/api/permissions';

const toast = useToast();

const permissions = ref<Permission[]>([]);
const loading = ref(true);

// Filters
const searchQuery = ref('');
const selectedApplication = ref<string | null>(null);
const selectedContext = ref<string | null>(null);
const selectedAction = ref<string | null>(null);

// Compute unique filter options
const applicationOptions = computed(() => {
  const unique = [...new Set(permissions.value.map(p => p.application))];
  return unique.sort().map(s => ({ label: s, value: s }));
});

const contextOptions = computed(() => {
  let filtered = permissions.value;
  if (selectedApplication.value) {
    filtered = filtered.filter(p => p.application === selectedApplication.value);
  }
  const unique = [...new Set(filtered.map(p => p.context))];
  return unique.sort().map(c => ({ label: c, value: c }));
});

const actionOptions = computed(() => [
  { label: 'view', value: 'view' },
  { label: 'create', value: 'create' },
  { label: 'update', value: 'update' },
  { label: 'delete', value: 'delete' },
  { label: 'retry', value: 'retry' },
]);

const hasActiveFilters = computed(() => {
  return searchQuery.value || selectedApplication.value || selectedContext.value || selectedAction.value;
});

const filteredPermissions = computed(() => {
  let result = permissions.value;

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase();
    result = result.filter(p =>
      p.permission.toLowerCase().includes(query) ||
      p.description?.toLowerCase().includes(query)
    );
  }

  if (selectedApplication.value) {
    result = result.filter(p => p.application === selectedApplication.value);
  }

  if (selectedContext.value) {
    result = result.filter(p => p.context === selectedContext.value);
  }

  if (selectedAction.value) {
    result = result.filter(p => p.action === selectedAction.value);
  }

  return result;
});

onMounted(async () => {
  await loadPermissions();
});

async function loadPermissions() {
  loading.value = true;
  try {
    const response = await permissionsApi.list();
    permissions.value = response.items;
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: e instanceof Error ? e.message : 'Failed to load permissions',
      life: 5000
    });
  } finally {
    loading.value = false;
  }
}

function clearFilters() {
  searchQuery.value = '';
  selectedApplication.value = null;
  selectedContext.value = null;
  selectedAction.value = null;
}

function getActionSeverity(action: string) {
  switch (action) {
    case 'view': return 'info';
    case 'create': return 'success';
    case 'update': return 'warn';
    case 'delete': return 'danger';
    default: return 'secondary';
  }
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Permissions</h1>
        <p class="page-subtitle">View all available permissions in the system</p>
      </div>
    </header>

    <!-- Filters -->
    <div class="fc-card filter-card">
      <div class="filter-row">
        <div class="filter-group">
          <label>Search</label>
          <span class="p-input-icon-left">
            <i class="pi pi-search" />
            <InputText
              v-model="searchQuery"
              placeholder="Search permissions..."
              class="filter-input"
            />
          </span>
        </div>

        <div class="filter-group">
          <label>Application</label>
          <Select
            v-model="selectedApplication"
            :options="applicationOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Applications"
            :showClear="true"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Context</label>
          <Select
            v-model="selectedContext"
            :options="contextOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Contexts"
            :showClear="true"
            class="filter-input"
          />
        </div>

        <div class="filter-group">
          <label>Action</label>
          <Select
            v-model="selectedAction"
            :options="actionOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Actions"
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
      <div v-if="loading" class="loading-container">
        <ProgressSpinner strokeWidth="3" />
      </div>

      <DataTable
        v-else
        :value="filteredPermissions"
        :paginator="true"
        :rows="20"
        :rowsPerPageOptions="[10, 20, 50, 100]"
        :showCurrentPageReport="true"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} permissions"
        class="p-datatable-sm"
      >
        <Column header="Permission" style="width: 35%">
          <template #body="{ data }">
            <span class="permission-string">{{ data.permission }}</span>
          </template>
        </Column>

        <Column field="application" header="Application" style="width: 12%">
          <template #body="{ data }">
            <Tag :value="data.application" severity="secondary" />
          </template>
        </Column>

        <Column field="context" header="Context" style="width: 12%">
          <template #body="{ data }">
            <span>{{ data.context }}</span>
          </template>
        </Column>

        <Column field="aggregate" header="Aggregate" style="width: 12%">
          <template #body="{ data }">
            <span>{{ data.aggregate }}</span>
          </template>
        </Column>

        <Column field="action" header="Action" style="width: 10%">
          <template #body="{ data }">
            <Tag :value="data.action" :severity="getActionSeverity(data.action)" />
          </template>
        </Column>

        <Column field="description" header="Description" style="width: 19%">
          <template #body="{ data }">
            <span class="description-text" v-tooltip.top="data.description">
              {{ data.description || '—' }}
            </span>
          </template>
        </Column>

        <template #empty>
          <div class="empty-message">
            <i class="pi pi-lock"></i>
            <span>No permissions found</span>
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
  min-width: 160px;
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

.permission-string {
  font-family: monospace;
  font-size: 13px;
  color: #475569;
}

.description-text {
  color: #64748b;
  font-size: 13px;
  display: block;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

:deep(.p-datatable .p-datatable-thead > tr > th) {
  background: #f8fafc;
  color: #475569;
  font-weight: 600;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
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
