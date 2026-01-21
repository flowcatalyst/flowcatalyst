<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Tag from 'primevue/tag';
import Select from 'primevue/select';
import ProgressSpinner from 'primevue/progressspinner';
import Message from 'primevue/message';
import { subscriptionsApi, type Subscription, type SubscriptionStatus } from '@/api/subscriptions';

const router = useRouter();
const subscriptions = ref<Subscription[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const searchQuery = ref('');
const statusFilter = ref<SubscriptionStatus | null>(null);

const statusOptions = [
  { label: 'All Statuses', value: null },
  { label: 'Active', value: 'ACTIVE' },
  { label: 'Paused', value: 'PAUSED' },
];

const filteredSubscriptions = computed(() => {
  let result = subscriptions.value;

  if (statusFilter.value) {
    result = result.filter(sub => sub.status === statusFilter.value);
  }

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase();
    result = result.filter(sub =>
      sub.code.toLowerCase().includes(query) ||
      sub.name.toLowerCase().includes(query) ||
      sub.target.toLowerCase().includes(query) ||
      sub.clientIdentifier?.toLowerCase().includes(query)
    );
  }

  return result;
});

onMounted(async () => {
  await loadSubscriptions();
});

async function loadSubscriptions() {
  loading.value = true;
  error.value = null;
  try {
    const response = await subscriptionsApi.list();
    subscriptions.value = response.subscriptions;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load subscriptions';
  } finally {
    loading.value = false;
  }
}

function getStatusSeverity(status: SubscriptionStatus) {
  switch (status) {
    case 'ACTIVE': return 'success';
    case 'PAUSED': return 'warn';
    default: return 'secondary';
  }
}

function getModeLabel(mode: string) {
  switch (mode) {
    case 'IMMEDIATE': return 'Immediate';
    case 'NEXT_ON_ERROR': return 'Next on Error';
    case 'BLOCK_ON_ERROR': return 'Block on Error';
    default: return mode;
  }
}

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleDateString();
}

function getScopeLabel(sub: Subscription) {
  if (sub.clientIdentifier) {
    return sub.clientIdentifier;
  }
  return 'Anchor-level';
}

function getEventTypesLabel(sub: Subscription) {
  const count = sub.eventTypes?.length || 0;
  return `${count} event type${count !== 1 ? 's' : ''}`;
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Subscriptions</h1>
        <p class="page-subtitle">Manage event subscriptions and webhook routing</p>
      </div>
      <Button label="Create Subscription" icon="pi pi-plus" @click="router.push('/subscriptions/new')" />
    </header>

    <Message v-if="error" severity="error" class="error-message">{{ error }}</Message>

    <div class="fc-card">
      <div class="toolbar">
        <span class="p-input-icon-left search-wrapper">
          <i class="pi pi-search" />
          <InputText v-model="searchQuery" placeholder="Search subscriptions..." />
        </span>
        <Select
          v-model="statusFilter"
          :options="statusOptions"
          optionLabel="label"
          optionValue="value"
          placeholder="Filter by status"
          class="status-filter"
        />
      </div>

      <div v-if="loading" class="loading-container">
        <ProgressSpinner strokeWidth="3" />
      </div>

      <DataTable
        v-else
        :value="filteredSubscriptions"
        paginator
        :rows="10"
        :rowsPerPageOptions="[10, 25, 50]"
        stripedRows
        emptyMessage="No subscriptions found"
      >
        <Column field="code" header="Code" sortable>
          <template #body="{ data }">
            <code class="sub-code">{{ data.code }}</code>
          </template>
        </Column>
        <Column field="name" header="Name" sortable />
        <Column header="Scope" sortable>
          <template #body="{ data }">
            <span class="scope-label">{{ getScopeLabel(data) }}</span>
          </template>
        </Column>
        <Column header="Event Types">
          <template #body="{ data }">
            <span class="event-types-count">{{ getEventTypesLabel(data) }}</span>
          </template>
        </Column>
        <Column field="dispatchPoolCode" header="Pool" sortable>
          <template #body="{ data }">
            <code class="pool-code">{{ data.dispatchPoolCode }}</code>
          </template>
        </Column>
        <Column header="Mode">
          <template #body="{ data }">
            <span class="mode-label">{{ getModeLabel(data.mode) }}</span>
          </template>
        </Column>
        <Column field="status" header="Status" sortable>
          <template #body="{ data }">
            <Tag :value="data.status" :severity="getStatusSeverity(data.status)" />
          </template>
        </Column>
        <Column field="createdAt" header="Created" sortable>
          <template #body="{ data }">
            {{ formatDate(data.createdAt) }}
          </template>
        </Column>
        <Column header="Actions" style="width: 120px">
          <template #body="{ data }">
            <Button
              icon="pi pi-eye"
              text
              rounded
              v-tooltip="'View'"
              @click="router.push(`/subscriptions/${data.id}`)"
            />
            <Button
              icon="pi pi-pencil"
              text
              rounded
              v-tooltip="'Edit'"
              @click="router.push(`/subscriptions/${data.id}`)"
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
  gap: 16px;
  margin-bottom: 16px;
}

.search-wrapper {
  position: relative;
  flex: 1;
}

.search-wrapper .pi-search {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: #94a3b8;
}

.search-wrapper input {
  padding-left: 36px;
  width: 100%;
}

.status-filter {
  min-width: 180px;
}

.loading-container {
  display: flex;
  justify-content: center;
  padding: 60px;
}

.error-message {
  margin-bottom: 16px;
}

.sub-code {
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 13px;
}

.pool-code {
  background: #e0f2fe;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  color: #0369a1;
}

.scope-label {
  font-size: 13px;
  color: #64748b;
}

.event-types-count {
  font-size: 13px;
}

.mode-label {
  font-size: 12px;
  color: #64748b;
}
</style>
