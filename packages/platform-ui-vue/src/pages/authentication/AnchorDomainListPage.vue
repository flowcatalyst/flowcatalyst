<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import ProgressSpinner from 'primevue/progressspinner';
import Message from 'primevue/message';
import Dialog from 'primevue/dialog';
import { useToast } from 'primevue/usetoast';
import { anchorDomainsApi, type AnchorDomain } from '@/api/anchor-domains';

const toast = useToast();
const domains = ref<AnchorDomain[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const searchQuery = ref('');

// Add domain dialog
const showAddDialog = ref(false);
const newDomain = ref('');
const addLoading = ref(false);
const addError = ref<string | null>(null);

// Delete confirmation dialog
const showDeleteDialog = ref(false);
const domainToDelete = ref<AnchorDomain | null>(null);
const deleteLoading = ref(false);

const filteredDomains = computed(() => {
  if (!searchQuery.value) return domains.value;
  const query = searchQuery.value.toLowerCase();
  return domains.value.filter(domain =>
    domain.domain.toLowerCase().includes(query)
  );
});

onMounted(async () => {
  await loadDomains();
});

async function loadDomains() {
  loading.value = true;
  error.value = null;
  try {
    const response = await anchorDomainsApi.list();
    domains.value = response.domains;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load anchor domains';
  } finally {
    loading.value = false;
  }
}

function openAddDialog() {
  newDomain.value = '';
  addError.value = null;
  showAddDialog.value = true;
}

async function addDomain() {
  if (!newDomain.value.trim()) {
    addError.value = 'Domain is required';
    return;
  }

  addLoading.value = true;
  addError.value = null;

  try {
    const created = await anchorDomainsApi.create({ domain: newDomain.value.trim() });
    domains.value.push(created);
    showAddDialog.value = false;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: `Anchor domain "${created.domain}" added successfully`,
      life: 3000,
    });
  } catch (e: any) {
    addError.value = e?.error || e?.message || 'Failed to add anchor domain';
  } finally {
    addLoading.value = false;
  }
}

function confirmDelete(domain: AnchorDomain) {
  domainToDelete.value = domain;
  showDeleteDialog.value = true;
}

async function deleteDomain() {
  if (!domainToDelete.value) return;

  deleteLoading.value = true;

  try {
    const response = await anchorDomainsApi.delete(domainToDelete.value.id);
    domains.value = domains.value.filter(d => d.id !== domainToDelete.value?.id);
    showDeleteDialog.value = false;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: response.message,
      life: 5000,
    });
  } catch (e: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: e?.error || e?.message || 'Failed to delete anchor domain',
      life: 5000,
    });
  } finally {
    deleteLoading.value = false;
    domainToDelete.value = null;
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
        <h1 class="page-title">Anchor Domains</h1>
        <p class="page-subtitle">
          Manage platform operator domains. Users from anchor domains have access to all clients.
        </p>
      </div>
      <Button label="Add Domain" icon="pi pi-plus" @click="openAddDialog" />
    </header>

    <Message v-if="error" severity="error" class="error-message">{{ error }}</Message>

    <div class="fc-card">
      <div class="toolbar">
        <span class="p-input-icon-left search-wrapper">
          <i class="pi pi-search" />
          <InputText v-model="searchQuery" placeholder="Search domains..." />
        </span>
      </div>

      <div v-if="loading" class="loading-container">
        <ProgressSpinner strokeWidth="3" />
      </div>

      <DataTable
        v-else
        :value="filteredDomains"
        paginator
        :rows="10"
        :rowsPerPageOptions="[10, 25, 50]"
        stripedRows
        emptyMessage="No anchor domains configured"
      >
        <Column field="domain" header="Domain" sortable>
          <template #body="{ data }">
            <code class="domain-code">{{ data.domain }}</code>
          </template>
        </Column>
        <Column field="userCount" header="Users" sortable>
          <template #body="{ data }">
            <span :class="{ 'has-users': data.userCount > 0 }">
              {{ data.userCount }} user{{ data.userCount !== 1 ? 's' : '' }}
            </span>
          </template>
        </Column>
        <Column field="createdAt" header="Added" sortable>
          <template #body="{ data }">
            {{ formatDate(data.createdAt) }}
          </template>
        </Column>
        <Column header="Actions" style="width: 100px">
          <template #body="{ data }">
            <Button
              icon="pi pi-trash"
              text
              rounded
              severity="danger"
              v-tooltip="'Remove'"
              @click="confirmDelete(data)"
            />
          </template>
        </Column>
      </DataTable>
    </div>

    <!-- Add Domain Dialog -->
    <Dialog
      v-model:visible="showAddDialog"
      header="Add Anchor Domain"
      modal
      :style="{ width: '450px' }"
    >
      <div class="dialog-content">
        <Message v-if="addError" severity="error" :closable="false" class="dialog-error">
          {{ addError }}
        </Message>

        <p class="dialog-description">
          Enter the email domain to add as an anchor domain. All users with email addresses
          from this domain will have access to all clients.
        </p>

        <div class="field">
          <label for="domain">Domain</label>
          <InputText
            id="domain"
            v-model="newDomain"
            placeholder="e.g., flowcatalyst.tech"
            class="w-full"
            @keyup.enter="addDomain"
          />
        </div>

        <Message severity="warn" :closable="false" class="warning-message">
          <strong>Warning:</strong> Adding an anchor domain grants global access to all
          existing and future users from this domain.
        </Message>
      </div>

      <template #footer>
        <Button
          label="Cancel"
          text
          @click="showAddDialog = false"
          :disabled="addLoading"
        />
        <Button
          label="Add Domain"
          icon="pi pi-plus"
          @click="addDomain"
          :loading="addLoading"
        />
      </template>
    </Dialog>

    <!-- Delete Confirmation Dialog -->
    <Dialog
      v-model:visible="showDeleteDialog"
      header="Remove Anchor Domain"
      modal
      :style="{ width: '450px' }"
    >
      <div class="dialog-content">
        <p>
          Are you sure you want to remove <strong>{{ domainToDelete?.domain }}</strong>
          as an anchor domain?
        </p>

        <Message
          v-if="domainToDelete?.userCount && domainToDelete.userCount > 0"
          severity="warn"
          :closable="false"
          class="warning-message"
        >
          <strong>{{ domainToDelete.userCount }} user(s)</strong> from this domain will
          lose their global access to all clients.
        </Message>
      </div>

      <template #footer>
        <Button
          label="Cancel"
          text
          @click="showDeleteDialog = false"
          :disabled="deleteLoading"
        />
        <Button
          label="Remove"
          icon="pi pi-trash"
          severity="danger"
          @click="deleteDomain"
          :loading="deleteLoading"
        />
      </template>
    </Dialog>
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

.domain-code {
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 13px;
}

.has-users {
  font-weight: 500;
  color: var(--primary-color);
}

.dialog-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dialog-description {
  color: #64748b;
  margin: 0;
}

.dialog-error {
  margin: 0;
}

.warning-message {
  margin: 0;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.field label {
  font-weight: 500;
  color: #334155;
}

.w-full {
  width: 100%;
}
</style>
