<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Textarea from 'primevue/textarea';
import MultiSelect from 'primevue/multiselect';
import Tag from 'primevue/tag';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Dialog from 'primevue/dialog';
import ProgressSpinner from 'primevue/progressspinner';
import { serviceAccountsApi, type ServiceAccount, type RoleAssignment, type RolesAssignedResponse } from '@/api/service-accounts';
import { rolesApi, type Role } from '@/api/roles';
import { clientsApi, type Client } from '@/api/clients';

const router = useRouter();
const route = useRoute();
const toast = useToast();

const serviceAccountId = route.params.id as string;

const serviceAccount = ref<ServiceAccount | null>(null);
const clients = ref<Client[]>([]);
const roleAssignments = ref<RoleAssignment[]>([]);
const availableRoles = ref<Role[]>([]);
const loading = ref(true);
const saving = ref(false);

// Edit mode
const editMode = ref(false);
const editName = ref('');
const editDescription = ref('');
const editClientIds = ref<string[]>([]);

const clientOptions = computed(() => {
  return clients.value.map(c => ({
    label: c.name,
    value: c.id,
  }));
});

// Credentials dialogs
const showRegenerateTokenDialog = ref(false);
const showRegenerateSecretDialog = ref(false);
const newToken = ref<string | null>(null);
const newSecret = ref<string | null>(null);

// Role picker dialog
const showRolePickerDialog = ref(false);
const roleSearchQuery = ref('');
const selectedRoleNames = ref<Set<string>>(new Set());
const savingRoles = ref(false);

const filteredAvailableRoles = computed(() => {
  const query = roleSearchQuery.value.toLowerCase();
  return availableRoles.value.filter(r =>
    r.name.toLowerCase().includes(query) ||
    r.displayName?.toLowerCase().includes(query)
  );
});

const hasRoleChanges = computed(() => {
  const currentRoles = new Set(roleAssignments.value.map(r => r.roleName));
  if (currentRoles.size !== selectedRoleNames.value.size) return true;
  for (const role of currentRoles) {
    if (!selectedRoleNames.value.has(role)) return true;
  }
  return false;
});

onMounted(async () => {
  await Promise.all([loadServiceAccount(), loadClients(), loadAvailableRoles()]);
  if (serviceAccount.value) {
    await loadRoleAssignments();
    if (route.query.edit === 'true') {
      startEdit();
    }
  }
  loading.value = false;
});

async function loadServiceAccount() {
  try {
    serviceAccount.value = await serviceAccountsApi.get(serviceAccountId);
    editName.value = serviceAccount.value.name;
    editDescription.value = serviceAccount.value.description || '';
    editClientIds.value = serviceAccount.value.clientIds || [];
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to load service account',
      life: 5000
    });
    console.error('Failed to fetch service account:', error);
    router.push('/identity/service-accounts');
  }
}

async function loadClients() {
  try {
    const response = await clientsApi.list();
    clients.value = response.clients;
  } catch (error) {
    console.error('Failed to fetch clients:', error);
  }
}

async function loadAvailableRoles() {
  try {
    const response = await rolesApi.list();
    availableRoles.value = response.items;
  } catch (error) {
    console.error('Failed to fetch available roles:', error);
  }
}

async function loadRoleAssignments() {
  try {
    const response = await serviceAccountsApi.getRoles(serviceAccountId);
    roleAssignments.value = response.roles;
  } catch (error) {
    console.error('Failed to fetch role assignments:', error);
  }
}

function startEdit() {
  editName.value = serviceAccount.value?.name || '';
  editDescription.value = serviceAccount.value?.description || '';
  editClientIds.value = serviceAccount.value?.clientIds || [];
  editMode.value = true;
}

function cancelEdit() {
  editName.value = serviceAccount.value?.name || '';
  editDescription.value = serviceAccount.value?.description || '';
  editClientIds.value = serviceAccount.value?.clientIds || [];
  editMode.value = false;
}

async function saveServiceAccount() {
  if (!editName.value.trim()) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Name is required',
      life: 3000
    });
    return;
  }

  saving.value = true;
  try {
    await serviceAccountsApi.update(serviceAccountId, {
      name: editName.value,
      description: editDescription.value || undefined,
      clientIds: editClientIds.value
    });
    serviceAccount.value!.name = editName.value;
    serviceAccount.value!.description = editDescription.value;
    serviceAccount.value!.clientIds = editClientIds.value;
    editMode.value = false;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: 'Service account updated successfully',
      life: 3000
    });
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to update service account',
      life: 5000
    });
  } finally {
    saving.value = false;
  }
}

async function regenerateToken() {
  saving.value = true;
  try {
    const response = await serviceAccountsApi.regenerateToken(serviceAccountId);
    newToken.value = response.authToken;
    showRegenerateTokenDialog.value = true;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: 'Auth token regenerated',
      life: 3000
    });
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to regenerate token',
      life: 5000
    });
  } finally {
    saving.value = false;
  }
}

async function regenerateSecret() {
  saving.value = true;
  try {
    const response = await serviceAccountsApi.regenerateSecret(serviceAccountId);
    newSecret.value = response.signingSecret;
    showRegenerateSecretDialog.value = true;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: 'Signing secret regenerated',
      life: 3000
    });
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to regenerate secret',
      life: 5000
    });
  } finally {
    saving.value = false;
  }
}

function copyToClipboard(text: string, label: string) {
  navigator.clipboard.writeText(text);
  toast.add({
    severity: 'info',
    summary: 'Copied',
    detail: `${label} copied to clipboard`,
    life: 2000
  });
}

function openRolePicker() {
  selectedRoleNames.value = new Set(roleAssignments.value.map(r => r.roleName));
  roleSearchQuery.value = '';
  showRolePickerDialog.value = true;
}

function toggleRole(roleName: string) {
  if (selectedRoleNames.value.has(roleName)) {
    selectedRoleNames.value.delete(roleName);
  } else {
    selectedRoleNames.value.add(roleName);
  }
  selectedRoleNames.value = new Set(selectedRoleNames.value);
}

function removeSelectedRole(roleName: string) {
  selectedRoleNames.value.delete(roleName);
  selectedRoleNames.value = new Set(selectedRoleNames.value);
}

async function saveRoles() {
  savingRoles.value = true;
  try {
    const roles = Array.from(selectedRoleNames.value);
    const response: RolesAssignedResponse = await serviceAccountsApi.assignRoles(serviceAccountId, roles);
    roleAssignments.value = response.roles;
    if (serviceAccount.value) {
      serviceAccount.value.roles = roles;
    }
    showRolePickerDialog.value = false;

    const added = response.addedRoles.length;
    const removed = response.removedRoles.length;
    let detail = 'Roles updated';
    if (added > 0 && removed > 0) {
      detail = `Added ${added} role(s), removed ${removed} role(s)`;
    } else if (added > 0) {
      detail = `Added ${added} role(s)`;
    } else if (removed > 0) {
      detail = `Removed ${removed} role(s)`;
    }

    toast.add({
      severity: 'success',
      summary: 'Success',
      detail,
      life: 3000
    });
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to save roles',
      life: 5000
    });
  } finally {
    savingRoles.value = false;
  }
}

function getRoleDisplay(roleName: string) {
  const role = availableRoles.value.find(r => r.name === roleName);
  return {
    displayName: role?.displayName || roleName.split(':').pop() || roleName,
    fullName: roleName
  };
}

function getClientName(clientId: string): string {
  const client = clients.value.find(c => c.id === clientId);
  return client?.name || clientId;
}

function getClientNames(clientIds: string[]): string {
  if (!clientIds || clientIds.length === 0) return 'All clients (no restriction)';
  return clientIds.map(id => getClientName(id)).join(', ');
}

function formatDate(dateStr: string | null | undefined) {
  if (!dateStr) return '—';
  return new Date(dateStr).toLocaleDateString();
}

function goBack() {
  router.push('/identity/service-accounts');
}
</script>

<template>
  <div class="page-container">
    <div v-if="loading" class="loading-container">
      <ProgressSpinner strokeWidth="3" />
    </div>

    <template v-else-if="serviceAccount">
      <header class="page-header">
        <div class="header-left">
          <Button
            icon="pi pi-arrow-left"
            text
            rounded
            severity="secondary"
            @click="goBack"
            v-tooltip.right="'Back to service accounts'"
          />
          <div>
            <h1 class="page-title">{{ serviceAccount.name }}</h1>
            <p class="page-subtitle"><code>{{ serviceAccount.code }}</code></p>
          </div>
          <Tag
            :value="serviceAccount.active ? 'Active' : 'Inactive'"
            :severity="serviceAccount.active ? 'success' : 'danger'"
          />
        </div>
      </header>

      <!-- Service Account Information Card -->
      <div class="fc-card">
        <div class="card-header">
          <h2 class="card-title">Service Account Information</h2>
          <Button
            v-if="!editMode"
            label="Edit"
            icon="pi pi-pencil"
            text
            @click="startEdit"
          />
          <div v-else class="edit-actions">
            <Button label="Cancel" text @click="cancelEdit" />
            <Button label="Save" icon="pi pi-check" :loading="saving" @click="saveServiceAccount" />
          </div>
        </div>

        <div class="info-grid">
          <div class="info-item">
            <label>Name</label>
            <InputText v-if="editMode" v-model="editName" class="w-full" />
            <span v-else>{{ serviceAccount.name }}</span>
          </div>

          <div class="info-item">
            <label>Code</label>
            <code>{{ serviceAccount.code }}</code>
          </div>

          <div class="info-item span-2">
            <label>Description</label>
            <Textarea v-if="editMode" v-model="editDescription" rows="2" class="w-full" />
            <span v-else>{{ serviceAccount.description || '—' }}</span>
          </div>

          <div class="info-item span-2">
            <label>Client Access</label>
            <MultiSelect
              v-if="editMode"
              v-model="editClientIds"
              :options="clientOptions"
              optionLabel="label"
              optionValue="value"
              placeholder="All clients (no restriction)"
              display="chip"
              filter
              class="w-full"
            />
            <span v-else>{{ getClientNames(serviceAccount.clientIds) }}</span>
          </div>

          <div class="info-item">
            <label>Auth Type</label>
            <Tag :value="serviceAccount.authType || 'BEARER'" severity="secondary" />
          </div>

          <div class="info-item">
            <label>Created</label>
            <span>{{ formatDate(serviceAccount.createdAt) }}</span>
          </div>

          <div class="info-item">
            <label>Last Used</label>
            <span>{{ formatDate(serviceAccount.lastUsedAt) }}</span>
          </div>
        </div>
      </div>

      <!-- Webhook Credentials Card -->
      <div class="fc-card">
        <div class="card-header">
          <h2 class="card-title">Webhook Credentials</h2>
        </div>

        <div class="credentials-section">
          <p class="credentials-info">
            Credentials are encrypted and cannot be viewed. You can regenerate them if needed.
          </p>

          <div class="credentials-actions">
            <div class="credential-action">
              <span class="credential-label">Auth Token (Bearer)</span>
              <Button
                label="Regenerate Token"
                icon="pi pi-refresh"
                outlined
                :loading="saving"
                @click="regenerateToken"
              />
            </div>

            <div class="credential-action">
              <span class="credential-label">Signing Secret (HMAC-SHA256)</span>
              <Button
                label="Regenerate Secret"
                icon="pi pi-refresh"
                outlined
                :loading="saving"
                @click="regenerateSecret"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Roles Card -->
      <div class="fc-card">
        <div class="card-header">
          <h2 class="card-title">Roles</h2>
          <Button
            label="Manage Roles"
            icon="pi pi-pencil"
            text
            @click="openRolePicker"
          />
        </div>

        <div v-if="roleAssignments.length === 0" class="no-roles-notice">
          <p>No roles assigned to this service account.</p>
          <Button
            label="Assign Roles"
            icon="pi pi-plus"
            text
            @click="openRolePicker"
          />
        </div>

        <DataTable v-else :value="roleAssignments" class="p-datatable-sm">
          <Column field="roleName" header="Role">
            <template #body="{ data }">
              <div class="role-cell">
                <span class="role-name">{{ data.roleName.split(':').pop() }}</span>
                <span class="role-full-name">{{ data.roleName }}</span>
              </div>
            </template>
          </Column>
          <Column field="assignmentSource" header="Source">
            <template #body="{ data }">
              <Tag
                :value="data.assignmentSource"
                :severity="data.assignmentSource === 'MANUAL' ? 'info' : 'secondary'"
              />
            </template>
          </Column>
          <Column field="assignedAt" header="Assigned">
            <template #body="{ data }">
              {{ formatDate(data.assignedAt) }}
            </template>
          </Column>
        </DataTable>
      </div>
    </template>

    <!-- Regenerate Token Dialog -->
    <Dialog
      v-model:visible="showRegenerateTokenDialog"
      header="New Auth Token"
      :style="{ width: '500px' }"
      :modal="true"
    >
      <div class="credential-dialog">
        <p class="warning-text">
          <i class="pi pi-exclamation-triangle"></i>
          Copy this token now. It will not be shown again.
        </p>
        <div class="credential-value">
          <code>{{ newToken }}</code>
          <Button
            icon="pi pi-copy"
            text
            rounded
            @click="copyToClipboard(newToken!, 'Token')"
            v-tooltip.top="'Copy'"
          />
        </div>
      </div>
      <template #footer>
        <Button label="Done" @click="showRegenerateTokenDialog = false" />
      </template>
    </Dialog>

    <!-- Regenerate Secret Dialog -->
    <Dialog
      v-model:visible="showRegenerateSecretDialog"
      header="New Signing Secret"
      :style="{ width: '500px' }"
      :modal="true"
    >
      <div class="credential-dialog">
        <p class="warning-text">
          <i class="pi pi-exclamation-triangle"></i>
          Copy this secret now. It will not be shown again.
        </p>
        <div class="credential-value">
          <code>{{ newSecret }}</code>
          <Button
            icon="pi pi-copy"
            text
            rounded
            @click="copyToClipboard(newSecret!, 'Secret')"
            v-tooltip.top="'Copy'"
          />
        </div>
      </div>
      <template #footer>
        <Button label="Done" @click="showRegenerateSecretDialog = false" />
      </template>
    </Dialog>

    <!-- Role Picker Dialog -->
    <Dialog
      v-model:visible="showRolePickerDialog"
      header="Manage Roles"
      :style="{ width: '700px' }"
      :modal="true"
      :closable="!savingRoles"
    >
      <div class="role-picker">
        <div class="role-pane available-roles">
          <div class="pane-header">
            <h4>Available Roles</h4>
            <InputText v-model="roleSearchQuery" placeholder="Filter roles..." class="role-filter" />
          </div>
          <div class="role-list">
            <div
              v-for="role in filteredAvailableRoles"
              :key="role.name"
              class="role-item"
              :class="{ selected: selectedRoleNames.has(role.name) }"
              @click="toggleRole(role.name)"
            >
              <div class="role-item-content">
                <span class="role-display-name">{{ role.displayName || role.name }}</span>
                <span class="role-name-code">{{ role.name }}</span>
              </div>
              <i v-if="selectedRoleNames.has(role.name)" class="pi pi-check check-icon"></i>
            </div>
            <div v-if="filteredAvailableRoles.length === 0" class="no-results">No roles found</div>
          </div>
        </div>

        <div class="role-pane selected-roles">
          <div class="pane-header">
            <h4>Selected Roles ({{ selectedRoleNames.size }})</h4>
          </div>
          <div class="role-list">
            <div
              v-for="roleName in selectedRoleNames"
              :key="roleName"
              class="role-item selected-item"
            >
              <div class="role-item-content">
                <span class="role-display-name">{{ getRoleDisplay(roleName).displayName }}</span>
                <span class="role-name-code">{{ roleName }}</span>
              </div>
              <Button
                icon="pi pi-times"
                text
                rounded
                severity="danger"
                size="small"
                @click="removeSelectedRole(roleName)"
                v-tooltip.top="'Remove'"
              />
            </div>
            <div v-if="selectedRoleNames.size === 0" class="no-results">No roles selected</div>
          </div>
        </div>
      </div>

      <template #footer>
        <Button label="Cancel" text @click="showRolePickerDialog = false" :disabled="savingRoles" />
        <Button
          label="Save Roles"
          icon="pi pi-check"
          :disabled="!hasRoleChanges"
          :loading="savingRoles"
          @click="saveRoles"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.loading-container {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 60px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.fc-card {
  margin-bottom: 24px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
  margin: 0;
}

.edit-actions {
  display: flex;
  gap: 8px;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.info-item.span-2 {
  grid-column: span 2;
}

.info-item label {
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.info-item span,
.info-item code {
  font-size: 14px;
  color: #1e293b;
}

.credentials-section {
  padding: 16px;
  background: #f8fafc;
  border-radius: 8px;
}

.credentials-info {
  font-size: 14px;
  color: #64748b;
  margin: 0 0 16px 0;
}

.credentials-actions {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.credential-action {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
}

.credential-label {
  font-size: 14px;
  font-weight: 500;
  color: #1e293b;
}

.credential-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.warning-text {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #f59e0b;
  font-size: 14px;
  margin: 0;
}

.credential-value {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
}

.credential-value code {
  flex: 1;
  font-size: 12px;
  word-break: break-all;
}

.no-roles-notice {
  text-align: center;
  padding: 24px;
  color: #64748b;
}

.no-roles-notice p {
  margin: 0 0 12px 0;
}

.role-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.role-name {
  font-size: 14px;
  font-weight: 500;
  color: #1e293b;
}

.role-full-name {
  font-size: 12px;
  color: #64748b;
  font-family: monospace;
}

.w-full {
  width: 100%;
}

/* Dual-pane role picker styles */
.role-picker {
  display: flex;
  gap: 16px;
  min-height: 350px;
}

.role-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  overflow: hidden;
}

.pane-header {
  padding: 12px;
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
}

.pane-header h4 {
  margin: 0 0 8px 0;
  font-size: 13px;
  font-weight: 600;
  color: #475569;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.selected-roles .pane-header h4 {
  margin-bottom: 0;
}

.role-filter {
  width: 100%;
}

.role-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.role-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.15s;
}

.role-item:hover {
  background: #f1f5f9;
}

.role-item.selected {
  background: #eff6ff;
}

.role-item.selected-item {
  background: #f8fafc;
  cursor: default;
}

.role-item-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.role-item-content .role-display-name {
  font-size: 13px;
  font-weight: 500;
  color: #1e293b;
}

.role-item-content .role-name-code {
  font-size: 11px;
  color: #64748b;
  font-family: monospace;
}

.check-icon {
  color: #3b82f6;
  font-size: 14px;
}

.no-results {
  padding: 20px;
  text-align: center;
  color: #94a3b8;
  font-size: 13px;
}

@media (max-width: 768px) {
  .info-grid {
    grid-template-columns: 1fr;
  }

  .info-item.span-2 {
    grid-column: span 1;
  }

  .role-picker {
    flex-direction: column;
    min-height: 500px;
  }

  .role-pane {
    min-height: 200px;
  }
}
</style>
