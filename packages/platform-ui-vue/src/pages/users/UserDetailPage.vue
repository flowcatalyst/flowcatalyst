<script setup lang="ts">
import {ref, computed, onMounted} from 'vue';
import {useRouter, useRoute} from 'vue-router';
import {useToast} from 'primevue/usetoast';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Tag from 'primevue/tag';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import AutoComplete from 'primevue/autocomplete';
import Dialog from 'primevue/dialog';
import ProgressSpinner from 'primevue/progressspinner';
import {usersApi, type User, type ClientAccessGrant, type RoleAssignment, type RolesAssignedResponse} from '@/api/users';
import {clientsApi, type Client} from '@/api/clients';
import {rolesApi, type Role} from '@/api/roles';

const router = useRouter();
const route = useRoute();
const toast = useToast();

const userId = route.params.id as string;

const user = ref<User | null>(null);
const clients = ref<Client[]>([]);
const clientGrants = ref<ClientAccessGrant[]>([]);
const loading = ref(true);
const saving = ref(false);

// Edit mode
const editMode = ref(false);
const editName = ref('');

// Add client access dialog
const showAddClientDialog = ref(false);
const clientSearchQuery = ref('');
const selectedClient = ref<Client | null>(null);
const filteredClients = ref<Client[]>([]);

// Role management
const roleAssignments = ref<RoleAssignment[]>([]);
const availableRoles = ref<Role[]>([]);
const showRolePickerDialog = ref(false);
const roleSearchQuery = ref('');
const selectedRoleNames = ref<Set<string>>(new Set());
const savingRoles = ref(false);

const isAnchorUser = computed(() => user.value?.isAnchorUser ?? false);

const userType = computed(() => {
  if (!user.value) return null;

  // Use the explicit scope if available
  if (user.value.scope) {
    switch (user.value.scope) {
      case 'ANCHOR':
        return {label: 'Anchor', severity: 'warn', icon: 'pi pi-star'};
      case 'PARTNER':
        return {label: 'Partner', severity: 'info', icon: null};
      case 'CLIENT':
        return {label: 'Client', severity: 'secondary', icon: null};
    }
  }

  // Fallback to derived logic for backwards compatibility
  if (user.value.isAnchorUser) {
    return {label: 'Anchor', severity: 'warn', icon: 'pi pi-star'};
  }
  const grantedCount = clientGrants.value.length;
  if (grantedCount > 0 || !user.value.clientId) {
    return {label: 'Partner', severity: 'info', icon: null};
  }
  return {label: 'Client', severity: 'secondary', icon: null};
});

const homeClient = computed(() => {
  if (!user.value?.clientId) return null;
  return clients.value.find(c => c.id === user.value?.clientId);
});

const grantedClients = computed(() => {
  return clientGrants.value.map(g => {
    const client = clients.value.find(c => c.id === g.clientId);
    return {
      ...g,
      clientName: client?.name || g.clientId,
      clientIdentifier: client?.identifier || '',
    };
  });
});

const availableClients = computed(() => {
  const existingIds = new Set([
    user.value?.clientId,
    ...clientGrants.value.map(g => g.clientId),
  ]);
  return clients.value.filter(c => !existingIds.has(c.id));
});

// Roles filtered by search query for the picker
const filteredAvailableRoles = computed(() => {
  const query = roleSearchQuery.value.toLowerCase();
  return availableRoles.value.filter(r =>
      r.name.toLowerCase().includes(query) ||
      r.displayName?.toLowerCase().includes(query)
  );
});

// Check if there are unsaved changes in the role picker
const hasRoleChanges = computed(() => {
  const currentRoles = new Set(roleAssignments.value.map(r => r.roleName));
  if (currentRoles.size !== selectedRoleNames.value.size) return true;
  for (const role of currentRoles) {
    if (!selectedRoleNames.value.has(role)) return true;
  }
  return false;
});

onMounted(async () => {
  await Promise.all([loadUser(), loadClients(), loadAvailableRoles()]);
  if (user.value) {
    await Promise.all([loadClientGrants(), loadRoleAssignments()]);
    // Check if we should start in edit mode
    if (route.query.edit === 'true') {
      startEdit();
    }
  }
  loading.value = false;
});

async function loadUser() {
  try {
    user.value = await usersApi.get(userId);
    editName.value = user.value.name;
  } catch (error) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: 'Failed to load user',
      life: 5000
    });
    console.error('Failed to fetch user:', error);
    router.push('/users');
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

async function loadClientGrants() {
  try {
    const response = await usersApi.getClientAccess(userId);
    clientGrants.value = response.grants;
  } catch (error) {
    console.error('Failed to fetch client grants:', error);
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
    const response = await usersApi.getRoles(userId);
    roleAssignments.value = response.roles;
  } catch (error) {
    console.error('Failed to fetch role assignments:', error);
  }
}

function startEdit() {
  editName.value = user.value?.name || '';
  editMode.value = true;
}

function cancelEdit() {
  editName.value = user.value?.name || '';
  editMode.value = false;
}

async function saveUser() {
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
    await usersApi.update(userId, {name: editName.value});
    user.value!.name = editName.value;
    editMode.value = false;
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: 'User updated successfully',
      life: 3000
    });
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to update user',
      life: 5000
    });
  } finally {
    saving.value = false;
  }
}

async function toggleUserStatus() {
  if (!user.value) return;

  saving.value = true;
  try {
    if (user.value.active) {
      await usersApi.deactivate(userId);
      user.value.active = false;
      toast.add({
        severity: 'success',
        summary: 'Success',
        detail: 'User deactivated',
        life: 3000
      });
    } else {
      await usersApi.activate(userId);
      user.value.active = true;
      toast.add({
        severity: 'success',
        summary: 'Success',
        detail: 'User activated',
        life: 3000
      });
    }
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to update user status',
      life: 5000
    });
  } finally {
    saving.value = false;
  }
}

function searchClients(event: any) {
  const query = event.query.toLowerCase();
  filteredClients.value = availableClients.value.filter(c =>
      c.name.toLowerCase().includes(query) ||
      c.identifier?.toLowerCase().includes(query)
  );
}

async function grantClientAccess() {
  if (!selectedClient.value) return;

  saving.value = true;
  try {
    const grant = await usersApi.grantClientAccess(userId, selectedClient.value.id);
    clientGrants.value.push(grant);
    showAddClientDialog.value = false;
    selectedClient.value = null;
    clientSearchQuery.value = '';
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: 'Client access granted',
      life: 3000
    });
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to grant client access',
      life: 5000
    });
  } finally {
    saving.value = false;
  }
}

async function revokeClientAccess(clientId: string) {
  saving.value = true;
  try {
    await usersApi.revokeClientAccess(userId, clientId);
    clientGrants.value = clientGrants.value.filter(g => g.clientId !== clientId);
    toast.add({
      severity: 'success',
      summary: 'Success',
      detail: 'Client access revoked',
      life: 3000
    });
  } catch (error: any) {
    toast.add({
      severity: 'error',
      summary: 'Error',
      detail: error?.message || 'Failed to revoke client access',
      life: 5000
    });
  } finally {
    saving.value = false;
  }
}

function openRolePicker() {
  // Initialize selected roles from current assignments
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
  // Force reactivity update
  selectedRoleNames.value = new Set(selectedRoleNames.value);
}

function removeSelectedRole(roleName: string) {
  selectedRoleNames.value.delete(roleName);
  selectedRoleNames.value = new Set(selectedRoleNames.value);
}

function cancelRolePicker() {
  showRolePickerDialog.value = false;
}

async function saveRoles() {
  savingRoles.value = true;
  try {
    const roles = Array.from(selectedRoleNames.value);
    const response: RolesAssignedResponse = await usersApi.assignRoles(userId, roles);

    // Update role assignments from response
    roleAssignments.value = response.roles;

    // Update user.roles for display
    if (user.value) {
      user.value.roles = roles;
    }

    showRolePickerDialog.value = false;

    const added = response.added.length;
    const removed = response.removed.length;
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

// Get role display info from available roles
function getRoleDisplay(roleName: string) {
  const role = availableRoles.value.find(r => r.name === roleName);
  return {
    displayName: role?.displayName || roleName.split(':').pop() || roleName,
    fullName: roleName
  };
}

function formatDate(dateStr: string | null | undefined) {
  if (!dateStr) return '—';
  return new Date(dateStr).toLocaleDateString();
}

function goBack() {
  router.push('/users');
}
</script>

<template>
  <div class="page-container">
    <div v-if="loading" class="loading-container">
      <ProgressSpinner strokeWidth="3"/>
    </div>

    <template v-else-if="user">
      <header class="page-header">
        <div class="header-left">
          <Button
              icon="pi pi-arrow-left"
              text
              rounded
              severity="secondary"
              @click="goBack"
              v-tooltip.right="'Back to users'"
          />
          <div>
            <h1 class="page-title">{{ user.name }}</h1>
            <p class="page-subtitle">{{ user.email }}</p>
          </div>
          <Tag
              v-if="userType"
              :value="userType.label"
              :severity="userType.severity"
              :icon="userType.icon"
              class="type-tag"
          />
          <Tag
              :value="user.active ? 'Active' : 'Inactive'"
              :severity="user.active ? 'success' : 'danger'"
          />
        </div>
        <div class="header-right">
          <Button
              :label="user.active ? 'Deactivate' : 'Activate'"
              :icon="user.active ? 'pi pi-ban' : 'pi pi-check'"
              :severity="user.active ? 'danger' : 'success'"
              outlined
              :loading="saving"
              @click="toggleUserStatus"
          />
        </div>
      </header>

      <!-- User Information Card -->
      <div class="fc-card">
        <div class="card-header">
          <h2 class="card-title">User Information</h2>
          <Button
              v-if="!editMode"
              label="Edit"
              icon="pi pi-pencil"
              text
              @click="startEdit"
          />
          <div v-else class="edit-actions">
            <Button label="Cancel" text @click="cancelEdit"/>
            <Button label="Save" icon="pi pi-check" :loading="saving" @click="saveUser"/>
          </div>
        </div>

        <div class="info-grid">
          <div class="info-item">
            <label>Name</label>
            <InputText
                v-if="editMode"
                v-model="editName"
                class="w-full"
            />
            <span v-else>{{ user.name }}</span>
          </div>

          <div class="info-item">
            <label>Email</label>
            <span>{{ user.email || '—' }}</span>
          </div>

          <div class="info-item">
            <label>Authentication</label>
            <span>{{ user.idpType === 'INTERNAL' ? 'Internal' : user.idpType || '—' }}</span>
          </div>

          <div class="info-item">
            <label>Created</label>
            <span>{{ formatDate(user.createdAt) }}</span>
          </div>
        </div>
      </div>

      <!-- Client Access Card -->
      <div class="fc-card">
        <div class="card-header">
          <h2 class="card-title">Client Access</h2>
          <Button
              v-if="!isAnchorUser"
              label="Add Client"
              icon="pi pi-plus"
              text
              @click="showAddClientDialog = true"
          />
        </div>

        <div v-if="isAnchorUser" class="anchor-notice">
          <i class="pi pi-star"></i>
          <span>This user has an anchor domain email and automatically has access to all clients.</span>
        </div>

        <template v-else>
          <div v-if="homeClient" class="home-client-section">
            <h3 class="section-subtitle">Home Client</h3>
            <div class="client-item home">
              <div class="client-info">
                <span class="client-name">{{ homeClient.name }}</span>
                <span class="client-identifier">{{ homeClient.identifier }}</span>
              </div>
              <Tag value="Home" severity="secondary"/>
            </div>
          </div>

          <div v-if="!homeClient && grantedClients.length === 0" class="no-clients-notice">
            <p>This user has no client access configured.</p>
            <Button
                label="Grant Client Access"
                icon="pi pi-plus"
                text
                @click="showAddClientDialog = true"
            />
          </div>

          <div v-if="grantedClients.length > 0" class="granted-clients-section">
            <h3 class="section-subtitle">Granted Access</h3>
            <DataTable :value="grantedClients" class="p-datatable-sm">
              <Column field="clientName" header="Client">
                <template #body="{ data }">
                  <div class="client-cell">
                    <span class="client-name">{{ data.clientName }}</span>
                    <span class="client-identifier">{{ data.clientIdentifier }}</span>
                  </div>
                </template>
              </Column>
              <Column field="grantedAt" header="Granted">
                <template #body="{ data }">
                  {{ formatDate(data.grantedAt) }}
                </template>
              </Column>
              <Column header="" style="width: 80px">
                <template #body="{ data }">
                  <Button
                      icon="pi pi-trash"
                      text
                      rounded
                      severity="danger"
                      @click="revokeClientAccess(data.clientId)"
                      v-tooltip.top="'Revoke access'"
                  />
                </template>
              </Column>
            </DataTable>
          </div>
        </template>
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
          <p>No roles assigned to this user.</p>
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

    <!-- Add Client Dialog -->
    <Dialog
        v-model:visible="showAddClientDialog"
        header="Grant Client Access"
        :style="{width: '450px'}"
        :modal="true"
    >
      <div class="dialog-content">
        <label>Search for a client</label>
        <AutoComplete
            v-model="selectedClient"
            :suggestions="filteredClients"
            @complete="searchClients"
            optionLabel="name"
            placeholder="Type to search..."
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
      </div>

      <template #footer>
        <Button label="Cancel" text @click="showAddClientDialog = false"/>
        <Button
            label="Grant Access"
            icon="pi pi-check"
            :disabled="!selectedClient"
            :loading="saving"
            @click="grantClientAccess"
        />
      </template>
    </Dialog>

    <!-- Role Picker Dialog (Dual-Pane) -->
    <Dialog
        v-model:visible="showRolePickerDialog"
        header="Manage Roles"
        :style="{width: '700px'}"
        :modal="true"
        :closable="!savingRoles"
    >
      <div class="role-picker">
        <!-- Left Pane: Available Roles -->
        <div class="role-pane available-roles">
          <div class="pane-header">
            <h4>Available Roles</h4>
            <InputText
                v-model="roleSearchQuery"
                placeholder="Filter roles..."
                class="role-filter"
            />
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
            <div v-if="filteredAvailableRoles.length === 0" class="no-results">
              No roles found
            </div>
          </div>
        </div>

        <!-- Right Pane: Selected Roles -->
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
            <div v-if="selectedRoleNames.size === 0" class="no-results">
              No roles selected
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <Button label="Cancel" text @click="cancelRolePicker" :disabled="savingRoles"/>
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

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.type-tag {
  margin-left: 8px;
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

.info-item label {
  font-size: 12px;
  font-weight: 500;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.info-item span {
  font-size: 14px;
  color: #1e293b;
}

.anchor-notice {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  background: #fffbeb;
  border: 1px solid #fcd34d;
  border-radius: 8px;
  color: #92400e;
}

.anchor-notice i {
  font-size: 20px;
  color: #f59e0b;
}

.section-subtitle {
  font-size: 13px;
  font-weight: 600;
  color: #64748b;
  margin: 0 0 12px 0;
}

.home-client-section {
  margin-bottom: 20px;
}

.client-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px;
  background: #f8fafc;
  border-radius: 6px;
}

.client-item.home {
  border: 1px solid #e2e8f0;
}

.client-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.client-name {
  font-size: 14px;
  font-weight: 500;
  color: #1e293b;
}

.client-identifier {
  font-size: 12px;
  color: #64748b;
  font-family: monospace;
}

.client-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.no-clients-notice,
.no-roles-notice {
  text-align: center;
  padding: 24px;
  color: #64748b;
}

.no-clients-notice p,
.no-roles-notice p {
  margin: 0 0 12px 0;
}

.granted-clients-section {
  margin-top: 20px;
}

.roles-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.role-tag {
  font-size: 12px;
}

.dialog-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.dialog-content label {
  font-size: 13px;
  font-weight: 500;
  color: #475569;
}

.client-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0;
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

.role-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0;
}

.role-display-name {
  font-size: 14px;
  font-weight: 500;
  color: #1e293b;
}

.role-name-code {
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

.role-item.selected-item:hover {
  background: #f1f5f9;
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
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.check-icon {
  color: #3b82f6;
  font-size: 14px;
  flex-shrink: 0;
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

  .role-picker {
    flex-direction: column;
    min-height: 500px;
  }

  .role-pane {
    min-height: 200px;
  }
}
</style>
