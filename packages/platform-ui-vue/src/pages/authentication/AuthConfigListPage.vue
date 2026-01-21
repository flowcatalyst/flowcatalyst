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
import Dialog from 'primevue/dialog';
import Select from 'primevue/select';
import Checkbox from 'primevue/checkbox';
import AutoComplete from 'primevue/autocomplete';
import { useToast } from 'primevue/usetoast';
import { authConfigsApi, type AuthConfig, type AuthProvider, type AuthConfigType } from '@/api/auth-configs';
import { clientsApi, type Client } from '@/api/clients';
import SecretRefInput from '@/components/SecretRefInput.vue';

const router = useRouter();
const toast = useToast();
const configs = ref<AuthConfig[]>([]);
const clients = ref<Client[]>([]);
const loading = ref(true);
const error = ref<string | null>(null);
const searchQuery = ref('');

// Create dialog state
const showCreateDialog = ref(false);
const createForm = ref({
  emailDomain: '',
  configType: 'CLIENT' as AuthConfigType,
  authProvider: 'INTERNAL' as AuthProvider,
  selectedClient: null as Client | null,
  oidcIssuerUrl: '',
  oidcClientId: '',
  oidcClientSecretRef: '',
  oidcMultiTenant: false,
  oidcIssuerPattern: '',
});
const filteredClients = ref<Client[]>([]);
const createLoading = ref(false);
const createError = ref<string | null>(null);

// Delete dialog state
const showDeleteDialog = ref(false);
const configToDelete = ref<AuthConfig | null>(null);
const deleteLoading = ref(false);

const authProviderOptions = [
  { label: 'Internal (Password)', value: 'INTERNAL' },
  { label: 'OIDC (External IDP)', value: 'OIDC' },
];

const configTypeOptions = [
  { label: 'Client', value: 'CLIENT', description: 'Users bound to a specific client' },
  { label: 'Partner', value: 'PARTNER', description: 'Partner IDP with granted client access' },
  { label: 'Anchor', value: 'ANCHOR', description: 'Platform-wide access to all clients' },
];

const filteredConfigs = computed(() => {
  if (!searchQuery.value) return configs.value;
  const query = searchQuery.value.toLowerCase();
  return configs.value.filter(config =>
    config.emailDomain.toLowerCase().includes(query) ||
    config.authProvider.toLowerCase().includes(query) ||
    getClientName(config.clientId)?.toLowerCase().includes(query)
  );
});

function getClientName(clientId: string | null): string | null {
  if (!clientId) return null;
  const client = clients.value.find(c => c.id === clientId);
  return client?.name || null;
}

function searchClients(event: { query: string }) {
  const query = event.query.toLowerCase();
  filteredClients.value = clients.value.filter(c =>
    c.name.toLowerCase().includes(query) ||
    c.identifier.toLowerCase().includes(query)
  );
}

onMounted(async () => {
  await Promise.all([loadConfigs(), loadClients()]);
});

async function loadClients() {
  try {
    const response = await clientsApi.list('ACTIVE');
    clients.value = response.clients;
  } catch (e) {
    console.error('Failed to load clients:', e);
  }
}

async function loadConfigs() {
  loading.value = true;
  error.value = null;
  try {
    const response = await authConfigsApi.list();
    configs.value = response.configs;
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load auth configurations';
  } finally {
    loading.value = false;
  }
}

function openCreateDialog() {
  createForm.value = {
    emailDomain: '',
    configType: 'CLIENT',
    authProvider: 'INTERNAL',
    selectedClient: null,
    oidcIssuerUrl: '',
    oidcClientId: '',
    oidcClientSecretRef: '',
    oidcMultiTenant: false,
    oidcIssuerPattern: '',
  };
  createError.value = null;
  showCreateDialog.value = true;
}

async function createConfig() {
  if (!createForm.value.emailDomain.trim()) {
    createError.value = 'Email domain is required';
    return;
  }

  // Validate CLIENT type requires a client
  if (createForm.value.configType === 'CLIENT' && !createForm.value.selectedClient) {
    createError.value = 'CLIENT type requires a primary client';
    return;
  }

  createLoading.value = true;
  createError.value = null;

  // Extract client ID if a client is selected (TSID string, not number)
  const primaryClientId = createForm.value.selectedClient?.id || undefined;
  const configType = createForm.value.configType;

  try {
    let created: AuthConfig;

    if (createForm.value.authProvider === 'INTERNAL') {
      created = await authConfigsApi.createInternal({
        emailDomain: createForm.value.emailDomain.trim(),
        configType,
        primaryClientId,
      });
    } else {
      if (!createForm.value.oidcIssuerUrl.trim()) {
        createError.value = 'OIDC Issuer URL is required';
        createLoading.value = false;
        return;
      }
      if (!createForm.value.oidcClientId.trim()) {
        createError.value = 'OIDC Client ID is required';
        createLoading.value = false;
        return;
      }

      created = await authConfigsApi.createOidc({
        emailDomain: createForm.value.emailDomain.trim(),
        configType,
        primaryClientId,
        oidcIssuerUrl: createForm.value.oidcIssuerUrl.trim(),
        oidcClientId: createForm.value.oidcClientId.trim(),
        oidcClientSecretRef: createForm.value.oidcClientSecretRef.trim() || undefined,
        oidcMultiTenant: createForm.value.oidcMultiTenant,
        oidcIssuerPattern: createForm.value.oidcIssuerPattern.trim() || undefined,
      });
    }

    configs.value.push(created);
    showCreateDialog.value = false;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: `Auth configuration for "${created.emailDomain}" created successfully`,
      life: 3000,
    });
  } catch (e: any) {
    createError.value = e?.error || e?.message || 'Failed to create auth configuration';
  } finally {
    createLoading.value = false;
  }
}

function confirmDelete(config: AuthConfig) {
  configToDelete.value = config;
  showDeleteDialog.value = true;
}

async function deleteConfig() {
  if (!configToDelete.value) return;

  deleteLoading.value = true;

  try {
    await authConfigsApi.delete(configToDelete.value.id);
    configs.value = configs.value.filter(c => c.id !== configToDelete.value?.id);
    showDeleteDialog.value = false;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: `Auth configuration for "${configToDelete.value.emailDomain}" deleted`,
      life: 3000,
    });
  } catch (e: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: e?.error || e?.message || 'Failed to delete auth configuration',
      life: 5000,
    });
  } finally {
    deleteLoading.value = false;
    configToDelete.value = null;
  }
}

async function validateSecret(secretRef: string) {
  try {
    const result = await authConfigsApi.validateSecret(secretRef);
    if (result.valid) {
      toast.add({
        severity: 'success',
        summary: 'Valid',
        detail: result.message,
        life: 3000,
      });
    } else {
      toast.add({
        severity: 'error',
        summary: 'Invalid',
        detail: result.message,
        life: 5000,
      });
    }
  } catch (e: any) {
    toast.add({
      severity: 'error',
      summary: 'Validation Failed',
      detail: e?.error || e?.message || 'Could not validate secret reference',
      life: 5000,
    });
  }
}

function getProviderSeverity(provider: AuthProvider) {
  return provider === 'OIDC' ? 'info' : 'secondary';
}

function getProviderLabel(provider: AuthProvider) {
  return provider === 'OIDC' ? 'OIDC' : 'Internal';
}

function getConfigTypeSeverity(configType: AuthConfigType) {
  switch (configType) {
    case 'ANCHOR': return 'warn';
    case 'PARTNER': return 'info';
    case 'CLIENT': return 'success';
    default: return 'secondary';
  }
}

function getConfigTypeLabel(configType: AuthConfigType) {
  switch (configType) {
    case 'ANCHOR': return 'Anchor';
    case 'PARTNER': return 'Partner';
    case 'CLIENT': return 'Client';
    default: return configType;
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
        <h1 class="page-title">Domain IDPs</h1>
        <p class="page-subtitle">
          Configure authentication methods for email domains. Choose between internal
          passwords or external OIDC identity providers.
        </p>
      </div>
      <Button label="Add Configuration" icon="pi pi-plus" @click="openCreateDialog" />
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
        :value="filteredConfigs"
        paginator
        :rows="10"
        :rowsPerPageOptions="[10, 25, 50]"
        stripedRows
        emptyMessage="No auth configurations found"
      >
        <Column field="emailDomain" header="Email Domain" sortable>
          <template #body="{ data }">
            <code class="domain-code">{{ data.emailDomain }}</code>
          </template>
        </Column>
        <Column field="authProvider" header="Auth Provider" sortable>
          <template #body="{ data }">
            <Tag
              :value="getProviderLabel(data.authProvider)"
              :severity="getProviderSeverity(data.authProvider)"
            />
          </template>
        </Column>
        <Column field="configType" header="Type" sortable>
          <template #body="{ data }">
            <Tag
              :value="getConfigTypeLabel(data.configType)"
              :severity="getConfigTypeSeverity(data.configType)"
            />
          </template>
        </Column>
        <Column field="primaryClientId" header="Primary Client" sortable>
          <template #body="{ data }">
            <span v-if="data.primaryClientId" class="client-name">
              {{ getClientName(data.primaryClientId) || data.primaryClientId }}
            </span>
            <span v-else class="text-muted">-</span>
          </template>
        </Column>
        <Column header="OIDC Issuer" sortable>
          <template #body="{ data }">
            <span v-if="data.oidcIssuerUrl" class="oidc-issuer">
              {{ data.oidcIssuerUrl }}
            </span>
            <span v-else class="text-muted">-</span>
          </template>
        </Column>
        <Column header="Secret" style="width: 100px">
          <template #body="{ data }">
            <i
              v-if="data.hasClientSecret"
              class="pi pi-check-circle text-success"
              v-tooltip="'Client secret configured'"
            />
            <span v-else class="text-muted">-</span>
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
              v-tooltip="'View Details'"
              @click="router.push(`/authentication/domain-idps/${data.id}`)"
            />
            <Button
              icon="pi pi-trash"
              text
              rounded
              severity="danger"
              v-tooltip="'Delete'"
              @click="confirmDelete(data)"
            />
          </template>
        </Column>
      </DataTable>
    </div>

    <!-- Create Config Dialog -->
    <Dialog
      v-model:visible="showCreateDialog"
      header="Add Auth Configuration"
      modal
      :style="{ width: '550px' }"
    >
      <div class="dialog-content">
        <Message v-if="createError" severity="error" :closable="false" class="dialog-error">
          {{ createError }}
        </Message>

        <div class="field">
          <label for="emailDomain">Email Domain</label>
          <InputText
            id="emailDomain"
            v-model="createForm.emailDomain"
            placeholder="e.g., acmecorp.com"
            class="w-full"
          />
          <small class="field-help">Users with this email domain will use this auth method</small>
        </div>

        <div class="field">
          <label for="configType">Configuration Type</label>
          <Select
            id="configType"
            v-model="createForm.configType"
            :options="configTypeOptions"
            optionLabel="label"
            optionValue="value"
            class="w-full"
          >
            <template #option="slotProps">
              <div class="config-type-option">
                <span class="type-label">{{ slotProps.option.label }}</span>
                <span class="type-description">{{ slotProps.option.description }}</span>
              </div>
            </template>
          </Select>
          <small class="field-help">
            <template v-if="createForm.configType === 'ANCHOR'">
              Users will have platform-wide access to all clients
            </template>
            <template v-else-if="createForm.configType === 'PARTNER'">
              Partner users with access to specific granted clients
            </template>
            <template v-else>
              Users will be bound to a specific primary client
            </template>
          </small>
        </div>

        <div class="field">
          <label for="authProvider">Authentication Method</label>
          <Select
            id="authProvider"
            v-model="createForm.authProvider"
            :options="authProviderOptions"
            optionLabel="label"
            optionValue="value"
            class="w-full"
          />
        </div>

        <div v-if="createForm.configType === 'CLIENT'" class="field">
          <label for="clientBinding">Primary Client</label>
          <AutoComplete
            id="clientBinding"
            v-model="createForm.selectedClient"
            :suggestions="filteredClients"
            @complete="searchClients"
            optionLabel="name"
            placeholder="Search for a client"
            class="w-full"
            dropdown
          >
            <template #option="slotProps">
              <div class="client-option">
                <span class="client-name">{{ slotProps.option.name }}</span>
                <span class="client-identifier">{{ slotProps.option.identifier }}</span>
              </div>
            </template>
          </AutoComplete>
          <small class="field-help">
            Users from this domain will be bound to this client
          </small>
        </div>

        <template v-if="createForm.authProvider === 'OIDC'">
          <div class="field">
            <label for="oidcIssuerUrl">OIDC Issuer URL</label>
            <InputText
              id="oidcIssuerUrl"
              v-model="createForm.oidcIssuerUrl"
              placeholder="https://auth.customer.com/realms/main"
              class="w-full"
            />
          </div>

          <div class="field">
            <label for="oidcClientId">OIDC Client ID</label>
            <InputText
              id="oidcClientId"
              v-model="createForm.oidcClientId"
              placeholder="flowcatalyst-client"
              class="w-full"
            />
          </div>

          <SecretRefInput
            v-model="createForm.oidcClientSecretRef"
            label="Client Secret (Optional)"
            @validate="validateSecret"
          />

          <div class="field checkbox-field">
            <Checkbox
              id="oidcMultiTenant"
              v-model="createForm.oidcMultiTenant"
              :binary="true"
            />
            <label for="oidcMultiTenant" class="checkbox-label">Multi-Tenant Configuration</label>
          </div>
          <small class="multi-tenant-help">
            Enable for multi-tenant IDPs like Microsoft Entra ID where each tenant has a different issuer URL
          </small>

          <div v-if="createForm.oidcMultiTenant" class="field">
            <label for="oidcIssuerPattern">Issuer Pattern (Optional)</label>
            <InputText
              id="oidcIssuerPattern"
              v-model="createForm.oidcIssuerPattern"
              placeholder="https://login.microsoftonline.com/{tenantId}/v2.0"
              class="w-full"
            />
            <small class="field-help">
              Use {tenantId} as placeholder. Leave empty to auto-derive from issuer URL.
            </small>
          </div>
        </template>
      </div>

      <template #footer>
        <Button
          label="Cancel"
          text
          @click="showCreateDialog = false"
          :disabled="createLoading"
        />
        <Button
          label="Create"
          icon="pi pi-plus"
          @click="createConfig"
          :loading="createLoading"
        />
      </template>
    </Dialog>

    <!-- Delete Confirmation Dialog -->
    <Dialog
      v-model:visible="showDeleteDialog"
      header="Delete Auth Configuration"
      modal
      :style="{ width: '450px' }"
    >
      <div class="dialog-content">
        <p>
          Are you sure you want to delete the auth configuration for
          <strong>{{ configToDelete?.emailDomain }}</strong>?
        </p>

        <Message severity="warn" :closable="false" class="warning-message">
          Users from this domain will no longer be able to authenticate until a new
          configuration is created.
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
          label="Delete"
          icon="pi pi-trash"
          severity="danger"
          @click="deleteConfig"
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

.oidc-issuer {
  font-size: 12px;
  color: #64748b;
  max-width: 250px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}

.text-muted {
  color: #94a3b8;
}

.text-success {
  color: #22c55e;
}

.client-name {
  font-weight: 500;
  color: #1e293b;
}

.client-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0;
}

.client-option .client-name {
  font-size: 14px;
}

.client-option .client-identifier {
  font-size: 12px;
  color: #64748b;
  font-family: monospace;
}

.config-type-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0;
}

.config-type-option .type-label {
  font-size: 14px;
  font-weight: 500;
}

.config-type-option .type-description {
  font-size: 12px;
  color: #64748b;
}

.dialog-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
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

.field-help {
  color: #64748b;
  font-size: 12px;
}

.checkbox-field {
  flex-direction: row;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.checkbox-label {
  margin: 0;
  cursor: pointer;
}

.multi-tenant-help {
  color: #64748b;
  font-size: 12px;
  margin-top: -8px;
  margin-bottom: 8px;
}

.w-full {
  width: 100%;
}
</style>
