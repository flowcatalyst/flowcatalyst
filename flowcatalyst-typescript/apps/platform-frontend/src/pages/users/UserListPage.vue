<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Select from 'primevue/select';
import Tag from 'primevue/tag';
import ProgressSpinner from 'primevue/progressspinner';
import { usersApi, type User } from '@/api/users';
import { clientsApi, type Client } from '@/api/clients';

const router = useRouter();
const toast = useToast();

const users = ref<User[]>([]);
const clients = ref<Client[]>([]);
const loading = ref(false);
const initialLoading = ref(true);
const totalRecords = ref(0);

// Filters
const searchQuery = ref('');
const selectedClientId = ref<string | null>(null);
const selectedStatus = ref<string | null>(null);

// Pagination
const page = ref(0);
const pageSize = ref(20);

const statusOptions = [
  { label: 'Active', value: 'active' },
  { label: 'Inactive', value: 'inactive' },
];

const clientOptions = computed(() => {
  return clients.value.map((c) => ({
    label: c.name,
    value: c.id,
  }));
});

const hasActiveFilters = computed(() => {
  return searchQuery.value || selectedClientId.value || selectedStatus.value;
});

onMounted(async () => {
  await Promise.all([loadUsers(), loadClients()]);
});

// Reload users when filters change (server-side filters)
watch([selectedClientId, selectedStatus], () => {
  page.value = 0;
  loadUsers();
});

// Debounced search
let searchTimeout: ReturnType<typeof setTimeout> | null = null;
watch(searchQuery, () => {
  if (searchTimeout) clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    page.value = 0;
    loadUsers();
  }, 300);
});

async function loadUsers() {
  loading.value = true;
  try {
    const response = await usersApi.list({
      type: 'USER',
      clientId: selectedClientId.value || undefined,
      active:
        selectedStatus.value === 'active'
          ? true
          : selectedStatus.value === 'inactive'
            ? false
            : undefined,
      q: searchQuery.value || undefined,
      page: page.value,
      pageSize: pageSize.value,
    });
    users.value = response.principals;
    totalRecords.value = response.total;
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to load users',
      life: 5000,
    });
    console.error('Failed to fetch users:', error);
  } finally {
    loading.value = false;
    initialLoading.value = false;
  }
}

function onPage(event: { page: number; rows: number }) {
  page.value = event.page;
  pageSize.value = event.rows;
  loadUsers();
}

async function loadClients() {
  try {
    const response = await clientsApi.list();
    clients.value = response.clients;
  } catch (error) {
    console.error('Failed to fetch clients:', error);
  }
}

function clearFilters() {
  searchQuery.value = '';
  selectedClientId.value = null;
  selectedStatus.value = null;
  page.value = 0;
  loadUsers();
}

function addUser() {
  router.push('/users/new');
}

function viewUser(user: User) {
  router.push(`/users/${user.id}`);
}

function editUser(user: User) {
  router.push(`/users/${user.id}?edit=true`);
}

function getClientName(clientId: string | null): string {
  if (!clientId) return 'No Client';
  const client = clients.value.find((c) => c.id === clientId);
  return client?.name || clientId;
}

// Determine user type based on access pattern
function getUserType(user: User): { label: string; severity: string; tooltip: string } {
  if (user.isAnchorUser) {
    return {
      label: 'Anchor',
      severity: 'warn',
      tooltip: 'Has access to all clients via anchor domain',
    };
  }

  const grantedCount = user.grantedClientIds?.length || 0;

  if (grantedCount > 0 || (!user.clientId && grantedCount === 0)) {
    // Has grants to multiple clients OR no home client = Partner
    return {
      label: 'Partner',
      severity: 'info',
      tooltip: user.clientId
        ? `Home: ${getClientName(user.clientId)}, +${grantedCount} granted`
        : `Access to ${grantedCount} client(s)`,
    };
  }

  // Has a home client only
  return {
    label: 'Client',
    severity: 'secondary',
    tooltip: `Home client: ${getClientName(user.clientId)}`,
  };
}

function formatDate(dateStr: string | undefined | null) {
  if (!dateStr) return '—';
  return new Date(dateStr).toLocaleDateString();
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Users</h1>
        <p class="page-subtitle">Manage platform users and their access</p>
      </div>
      <Button label="Add User" icon="pi pi-user-plus" @click="addUser" />
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
              placeholder="Search by name or email..."
              class="filter-input"
            />
          </span>
        </div>

        <div class="filter-group">
          <label>Client</label>
          <Select
            v-model="selectedClientId"
            :options="clientOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="All Clients"
            :showClear="true"
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
        :value="users"
        :loading="loading"
        :paginator="true"
        :rows="pageSize"
        :totalRecords="totalRecords"
        :rowsPerPageOptions="[10, 20, 50]"
        :lazy="true"
        :showCurrentPageReport="true"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} users"
        stripedRows
        class="p-datatable-sm"
        @page="onPage"
      >
        <Column field="name" header="Name" sortable style="width: 20%">
          <template #body="{ data }">
            <span class="user-name">{{ data.name }}</span>
          </template>
        </Column>

        <Column field="email" header="Email" sortable style="width: 25%">
          <template #body="{ data }">
            <span class="user-email">{{ data.email || '—' }}</span>
          </template>
        </Column>

        <Column header="Type" style="width: 12%">
          <template #body="{ data }">
            <Tag
              :value="getUserType(data).label"
              :severity="getUserType(data).severity"
              :icon="data.isAnchorUser ? 'pi pi-star' : undefined"
              v-tooltip.top="getUserType(data).tooltip"
            />
          </template>
        </Column>

        <Column header="Client" style="width: 15%">
          <template #body="{ data }">
            <div class="client-cell">
              <span v-if="data.isAnchorUser" class="all-clients-text">All Clients</span>
              <template v-else-if="data.clientId">
                <!-- User has a home client -->
                <span class="client-name-text">{{ getClientName(data.clientId) }}</span>
                <span v-if="data.grantedClientIds?.length > 0" class="additional-clients">
                  +{{ data.grantedClientIds.length }} more
                </span>
              </template>
              <template v-else-if="data.grantedClientIds?.length > 0">
                <!-- No home client but has granted access -->
                <span class="client-name-text">{{ getClientName(data.grantedClientIds[0]) }}</span>
                <span v-if="data.grantedClientIds.length > 1" class="additional-clients">
                  +{{ data.grantedClientIds.length - 1 }} more
                </span>
              </template>
              <template v-else>
                <!-- No home client and no grants -->
                <span class="no-client-text">No Client</span>
              </template>
            </div>
          </template>
        </Column>

        <Column field="active" header="Status" style="width: 10%">
          <template #body="{ data }">
            <Tag
              :value="data.active ? 'Active' : 'Inactive'"
              :severity="data.active ? 'success' : 'danger'"
            />
          </template>
        </Column>

        <Column field="roles" header="Roles" style="width: 15%">
          <template #body="{ data }">
            <div class="roles-container">
              <Tag
                v-for="role in (data.roles || []).slice(0, 2)"
                :key="role"
                :value="role.split(':').pop()"
                severity="secondary"
                class="role-tag"
              />
              <span v-if="(data.roles || []).length > 2" class="more-roles">
                +{{ data.roles.length - 2 }} more
              </span>
            </div>
          </template>
        </Column>

        <Column field="createdAt" header="Created" sortable style="width: 10%">
          <template #body="{ data }">
            <span class="date-text">{{ formatDate(data.createdAt) }}</span>
          </template>
        </Column>

        <Column header="Actions" style="width: 5%">
          <template #body="{ data }">
            <div class="action-buttons">
              <Button
                icon="pi pi-eye"
                text
                rounded
                severity="secondary"
                @click="viewUser(data)"
                v-tooltip.top="'View'"
              />
              <Button
                icon="pi pi-pencil"
                text
                rounded
                severity="secondary"
                @click="editUser(data)"
                v-tooltip.top="'Edit'"
              />
            </div>
          </template>
        </Column>

        <template #empty>
          <div class="empty-message">
            <i class="pi pi-users"></i>
            <span>No users found</span>
            <Button v-if="hasActiveFilters" label="Clear filters" link @click="clearFilters" />
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

.user-name {
  font-weight: 500;
  color: #1e293b;
}

.user-email {
  color: #64748b;
  font-size: 13px;
}

.client-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.client-name-text {
  font-size: 13px;
  color: #1e293b;
}

.all-clients-text {
  font-size: 13px;
  color: #f59e0b;
  font-weight: 500;
}

.no-client-text {
  font-size: 13px;
  color: #94a3b8;
  font-style: italic;
}

.additional-clients {
  font-size: 11px;
  color: #64748b;
}

.roles-container {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}

.role-tag {
  font-size: 11px;
}

.more-roles {
  font-size: 12px;
  color: #64748b;
}

.date-text {
  font-size: 13px;
  color: #64748b;
}

.action-buttons {
  display: flex;
  gap: 4px;
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
