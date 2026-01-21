<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Tag from 'primevue/tag';
import ProgressSpinner from 'primevue/progressspinner';
import Message from 'primevue/message';
import { clientsApi, type Client } from '@/api/clients';

const router = useRouter();
const clients = ref<Client[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const searchQuery = ref('');

const filteredClients = computed(() => {
  if (!searchQuery.value) return clients.value;
  const query = searchQuery.value.toLowerCase();
  return clients.value.filter(client =>
    client.identifier.toLowerCase().includes(query) ||
    client.name.toLowerCase().includes(query)
  );
});

onMounted(async () => {
  await loadClients();
});

async function loadClients() {
  loading.value = true;
  error.value = null;
  try {
    const response = await clientsApi.list();
    clients.value = response.clients;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load clients';
  } finally {
    loading.value = false;
  }
}

function getStatusSeverity(status: string) {
  switch (status) {
    case 'ACTIVE': return 'success';
    case 'SUSPENDED': return 'warn';
    case 'INACTIVE': return 'secondary';
    default: return 'secondary';
  }
}

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleDateString();
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Clients</h1>
        <p class="page-subtitle">Manage customer clients and their configurations</p>
      </div>
      <Button label="Create Client" icon="pi pi-plus" @click="router.push('/clients/new')" />
    </header>

    <Message v-if="error" severity="error" class="error-message">{{ error }}</Message>

    <div class="fc-card">
      <div class="toolbar">
        <span class="p-input-icon-left search-wrapper">
          <i class="pi pi-search" />
          <InputText v-model="searchQuery" placeholder="Search clients..." />
        </span>
      </div>

      <div v-if="loading" class="loading-container">
        <ProgressSpinner strokeWidth="3" />
      </div>

      <DataTable
        v-else
        :value="filteredClients"
        paginator
        :rows="10"
        :rowsPerPageOptions="[10, 25, 50]"
        stripedRows
        emptyMessage="No clients found"
      >
        <Column field="identifier" header="Identifier" sortable>
          <template #body="{ data }">
            <code class="client-code">{{ data.identifier }}</code>
          </template>
        </Column>
        <Column field="name" header="Name" sortable />
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
              @click="router.push(`/clients/${data.id}`)"
            />
            <Button
              icon="pi pi-pencil"
              text
              rounded
              v-tooltip="'Edit'"
              @click="router.push(`/clients/${data.id}`)"
            />
          </template>
        </Column>
      </DataTable>
    </div>
  </div>
</template>

<style scoped>
.toolbar {
  margin-bottom: 16px;
}

.search-wrapper {
  position: relative;
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
}

.loading-container {
  display: flex;
  justify-content: center;
  padding: 60px;
}

.error-message {
  margin-bottom: 16px;
}

.client-code {
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 13px;
}
</style>
