<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import Select from 'primevue/select';
import MultiSelect from 'primevue/multiselect';
import Checkbox from 'primevue/checkbox';
import Chip from 'primevue/chip';
import Message from 'primevue/message';
import Dialog from 'primevue/dialog';
import { useToast } from 'primevue/usetoast';
import { oauthClientsApi, type ClientType } from '@/api/oauth-clients';
import { applicationsApi, type Application } from '@/api/applications';

const router = useRouter();
const toast = useToast();

const applications = ref<Application[]>([]);
const loading = ref(false);
const error = ref<string | null>(null);

// Form state
const form = ref({
  clientName: '',
  clientType: 'PUBLIC' as ClientType,
  redirectUris: [] as string[],
  allowedOrigins: [] as string[],
  grantTypes: ['authorization_code', 'refresh_token'],
  defaultScopes: ['openid', 'profile', 'email'],
  pkceRequired: true,
  applicationIds: [] as string[],
});

const newRedirectUri = ref('');
const newAllowedOrigin = ref('');

// Secret dialog state
const showSecretDialog = ref(false);
const clientSecret = ref<string | null>(null);

const clientTypeOptions = [
  { label: 'Public (SPA, Mobile)', value: 'PUBLIC', description: 'No client secret, PKCE required' },
  { label: 'Confidential (Server)', value: 'CONFIDENTIAL', description: 'Has client secret' },
];

const grantTypeOptions = [
  { label: 'Authorization Code', value: 'authorization_code' },
  { label: 'Refresh Token', value: 'refresh_token' },
  { label: 'Client Credentials', value: 'client_credentials' },
];

const scopeOptions = [
  { label: 'openid', value: 'openid' },
  { label: 'profile', value: 'profile' },
  { label: 'email', value: 'email' },
  { label: 'offline_access', value: 'offline_access' },
];

const isValid = computed(() => {
  const valid = (
    form.value.clientName.trim() !== '' &&
    form.value.redirectUris.length > 0 &&
    form.value.grantTypes.length > 0
  );
  console.log('Form validation:', {
    clientName: form.value.clientName,
    redirectUris: form.value.redirectUris,
    grantTypes: form.value.grantTypes,
    isValid: valid
  });
  return valid;
});

onMounted(async () => {
  await loadApplications();
});

async function loadApplications() {
  try {
    // Only load user-facing applications (not integrations)
    // OAuth clients are associated with applications, not integrations
    const response = await applicationsApi.listApplicationsOnly(true);
    applications.value = response.applications || [];
    console.log('Loaded applications:', applications.value);
  } catch (e: any) {
    console.error('Failed to load applications:', e);
    toast.add({
      severity: 'warn',
      summary: 'Warning',
      detail: 'Could not load applications: ' + (e?.message || 'Unknown error'),
      life: 5000,
    });
  }
}

function addRedirectUri() {
  const uri = newRedirectUri.value.trim();
  if (uri && !form.value.redirectUris.includes(uri)) {
    // Basic URL validation
    try {
      new URL(uri);
      form.value.redirectUris.push(uri);
      newRedirectUri.value = '';
    } catch {
      toast.add({
        severity: 'error',
        summary: 'Invalid URL',
        detail: 'Please enter a valid URL',
        life: 3000,
      });
    }
  }
}

function removeRedirectUri(uri: string) {
  form.value.redirectUris = form.value.redirectUris.filter(u => u !== uri);
}

function addAllowedOrigin() {
  const origin = newAllowedOrigin.value.trim();
  if (origin && !form.value.allowedOrigins.includes(origin)) {
    // Basic URL validation - must be a valid origin (scheme + host)
    try {
      const url = new URL(origin);
      // Origin should not have path (other than /)
      if (url.pathname !== '/' && url.pathname !== '') {
        toast.add({
          severity: 'error',
          summary: 'Invalid Origin',
          detail: 'Origin should not include a path (e.g., https://example.com)',
          life: 3000,
        });
        return;
      }
      // Use the origin (scheme + host + port)
      form.value.allowedOrigins.push(url.origin);
      newAllowedOrigin.value = '';
    } catch {
      toast.add({
        severity: 'error',
        summary: 'Invalid URL',
        detail: 'Please enter a valid origin URL',
        life: 3000,
      });
    }
  }
}

function removeAllowedOrigin(origin: string) {
  form.value.allowedOrigins = form.value.allowedOrigins.filter(o => o !== origin);
}

async function createClient() {
  if (!isValid.value) return;

  loading.value = true;
  error.value = null;

  try {
    const response = await oauthClientsApi.create({
      clientName: form.value.clientName.trim(),
      clientType: form.value.clientType,
      redirectUris: form.value.redirectUris,
      allowedOrigins: form.value.allowedOrigins.length > 0 ? form.value.allowedOrigins : undefined,
      grantTypes: form.value.grantTypes,
      defaultScopes: form.value.defaultScopes,
      pkceRequired: form.value.pkceRequired,
      applicationIds: form.value.applicationIds.length > 0 ? form.value.applicationIds : undefined,
    });

    // If confidential client, show the secret
    if (response.clientSecret) {
      clientSecret.value = response.clientSecret;
      showSecretDialog.value = true;
    } else {
      toast.add({
        severity: 'success',
        summary: 'Success',
        detail: `OAuth client "${response.client.clientName}" created successfully`,
        life: 3000,
      });
      router.push(`/authentication/oauth-clients/${response.client.id}`);
    }
  } catch (e: any) {
    error.value = e?.error || e?.message || 'Failed to create OAuth client';
  } finally {
    loading.value = false;
  }
}

function copySecret() {
  if (clientSecret.value) {
    navigator.clipboard.writeText(clientSecret.value);
    toast.add({
      severity: 'success',
      summary: 'Copied',
      detail: 'Client secret copied to clipboard',
      life: 2000,
    });
  }
}

function closeSecretDialog() {
  showSecretDialog.value = false;
  router.push('/authentication/oauth-clients');
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <Button
          icon="pi pi-arrow-left"
          text
          class="back-button"
          @click="router.push('/authentication/oauth-clients')"
        />
        <h1 class="page-title">Create OAuth Client</h1>
        <p class="page-subtitle">
          Register a new OAuth2/OIDC client for applications that use FlowCatalyst as their identity provider.
        </p>
      </div>
    </header>

    <Message v-if="error" severity="error" class="error-message" :closable="true" @close="error = null">
      {{ error }}
    </Message>

    <div class="fc-card">
      <div class="form-content">
        <div class="field">
          <label for="clientName">Client Name *</label>
          <InputText
            id="clientName"
            v-model="form.clientName"
            placeholder="e.g., Production SPA, Development Server"
            class="w-full"
          />
          <small class="field-help">A human-readable name for this client</small>
        </div>

        <div class="field">
          <label for="clientType">Client Type *</label>
          <Select
            id="clientType"
            v-model="form.clientType"
            :options="clientTypeOptions"
            optionLabel="label"
            optionValue="value"
            class="w-full"
          >
            <template #option="slotProps">
              <div class="type-option">
                <span class="type-label">{{ slotProps.option.label }}</span>
                <span class="type-description">{{ slotProps.option.description }}</span>
              </div>
            </template>
          </Select>
        </div>

        <div class="field">
          <label>Redirect URIs *</label>
          <div class="redirect-uri-input">
            <InputText
              v-model="newRedirectUri"
              placeholder="https://app.example.com/callback"
              class="flex-grow"
              @keyup.enter="addRedirectUri"
            />
            <Button
              icon="pi pi-plus"
              @click="addRedirectUri"
              :disabled="!newRedirectUri.trim()"
            />
          </div>
          <div v-if="form.redirectUris.length > 0" class="uri-list">
            <Chip
              v-for="uri in form.redirectUris"
              :key="uri"
              :label="uri"
              removable
              @remove="removeRedirectUri(uri)"
            />
          </div>
          <small class="field-help">Allowed callback URLs for OAuth redirects. Must use HTTPS (except localhost).</small>
        </div>

        <div class="field">
          <label>Allowed CORS Origins</label>
          <div class="redirect-uri-input">
            <InputText
              v-model="newAllowedOrigin"
              placeholder="https://app.example.com"
              class="flex-grow"
              @keyup.enter="addAllowedOrigin"
            />
            <Button
              icon="pi pi-plus"
              @click="addAllowedOrigin"
              :disabled="!newAllowedOrigin.trim()"
            />
          </div>
          <div v-if="form.allowedOrigins.length > 0" class="uri-list">
            <Chip
              v-for="origin in form.allowedOrigins"
              :key="origin"
              :label="origin"
              removable
              @remove="removeAllowedOrigin(origin)"
            />
          </div>
          <small class="field-help">Origins allowed to make browser requests to the token endpoint. Must use HTTPS (except localhost).</small>
        </div>

        <div class="field">
          <label for="grantTypes">Grant Types *</label>
          <MultiSelect
            id="grantTypes"
            v-model="form.grantTypes"
            :options="grantTypeOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="Select grant types"
            class="w-full"
          />
        </div>

        <div class="field">
          <label for="defaultScopes">Default Scopes</label>
          <MultiSelect
            id="defaultScopes"
            v-model="form.defaultScopes"
            :options="scopeOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="Select scopes"
            class="w-full"
          />
        </div>

        <div class="field checkbox-field">
          <Checkbox
            id="pkceRequired"
            v-model="form.pkceRequired"
            :binary="true"
            :disabled="form.clientType === 'PUBLIC'"
          />
          <label for="pkceRequired" class="checkbox-label">Require PKCE</label>
        </div>
        <small v-if="form.clientType === 'PUBLIC'" class="field-help">
          PKCE is always required for public clients
        </small>

        <div class="field">
          <label for="applications">Associated Applications</label>
          <MultiSelect
            id="applications"
            v-model="form.applicationIds"
            :options="applications"
            optionLabel="name"
            optionValue="id"
            placeholder="Select applications (optional)"
            class="w-full"
            filter
          >
            <template #option="slotProps">
              <div class="app-option">
                <span class="app-name">{{ slotProps.option.name }}</span>
                <span class="app-code">{{ slotProps.option.code }}</span>
              </div>
            </template>
          </MultiSelect>
          <small class="field-help">
            Only users with access to these applications can authenticate. Leave empty for no restrictions.
          </small>
        </div>

        <div class="form-actions">
          <Button
            label="Cancel"
            text
            @click="router.push('/authentication/oauth-clients')"
            :disabled="loading"
          />
          <Button
            label="Create OAuth Client"
            icon="pi pi-plus"
            @click="createClient"
            :loading="loading"
            :disabled="!isValid"
          />
        </div>
      </div>
    </div>

    <!-- Client Secret Dialog -->
    <Dialog
      v-model:visible="showSecretDialog"
      header="Client Secret Generated"
      modal
      :closable="false"
      :style="{ width: '500px' }"
    >
      <div class="dialog-content">
        <Message severity="warn" :closable="false">
          Copy this secret now. It will not be shown again.
        </Message>

        <div class="secret-display">
          <code class="secret-code">{{ clientSecret }}</code>
          <Button
            icon="pi pi-copy"
            text
            v-tooltip="'Copy to clipboard'"
            @click="copySecret"
          />
        </div>
      </div>

      <template #footer>
        <Button
          label="I've copied the secret"
          icon="pi pi-check"
          @click="closeSecretDialog"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.back-button {
  margin-right: 8px;
}

.error-message {
  margin-bottom: 16px;
}

.form-content {
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 600px;
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
}

.checkbox-label {
  margin: 0;
  cursor: pointer;
}

.type-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0;
}

.type-option .type-label {
  font-size: 14px;
  font-weight: 500;
}

.type-option .type-description {
  font-size: 12px;
  color: #64748b;
}

.redirect-uri-input {
  display: flex;
  gap: 8px;
}

.flex-grow {
  flex: 1;
}

.uri-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.app-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 0;
}

.app-option .app-name {
  font-size: 14px;
  font-weight: 500;
}

.app-option .app-code {
  font-size: 12px;
  color: #64748b;
  font-family: monospace;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #e2e8f0;
}

.dialog-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.secret-display {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f8fafc;
  padding: 12px;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
}

.secret-code {
  flex: 1;
  font-size: 13px;
  word-break: break-all;
  color: #1e293b;
}

.w-full {
  width: 100%;
}
</style>
