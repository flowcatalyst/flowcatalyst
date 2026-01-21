<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import Button from 'primevue/button';
import InputText from 'primevue/inputtext';
import InputNumber from 'primevue/inputnumber';
import Textarea from 'primevue/textarea';
import Select from 'primevue/select';
import Checkbox from 'primevue/checkbox';
import MultiSelect from 'primevue/multiselect';
import Message from 'primevue/message';
import ClientSelect from '@/components/ClientSelect.vue';
import { subscriptionsApi, type SubscriptionMode, type EventTypeBinding } from '@/api/subscriptions';
import { dispatchPoolsApi, type DispatchPool } from '@/api/dispatch-pools';
import { eventTypesApi, type EventType } from '@/api/event-types';

const router = useRouter();
const toast = useToast();

// Form fields
const code = ref('');
const name = ref('');
const description = ref('');
const target = ref('');
const queue = ref('');
const maxAgeSeconds = ref<number>(86400);
const delaySeconds = ref<number>(0);
const sequence = ref<number>(99);
const timeoutSeconds = ref<number>(30);
const mode = ref<SubscriptionMode>('IMMEDIATE');
const dispatchPoolId = ref<string | null>(null);
const clientId = ref<string | null>(null);
const isAnchorLevel = ref(false);
const selectedEventTypes = ref<string[]>([]);

// Lookup data
const dispatchPools = ref<DispatchPool[]>([]);
const eventTypes = ref<EventType[]>([]);
const loadingPools = ref(true);
const loadingEventTypes = ref(true);
const submitting = ref(false);
const errorMessage = ref<string | null>(null);

const CODE_PATTERN = /^[a-z][a-z0-9-]*$/;

const modeOptions = [
  { label: 'Immediate', value: 'IMMEDIATE' },
  { label: 'Next on Error', value: 'NEXT_ON_ERROR' },
  { label: 'Block on Error', value: 'BLOCK_ON_ERROR' },
];

const isCodeValid = computed(() => {
  return !code.value || CODE_PATTERN.test(code.value);
});

const isTargetValid = computed(() => {
  if (!target.value) return true;
  try {
    const url = new URL(target.value);
    return url.protocol === 'http:' || url.protocol === 'https:';
  } catch {
    return false;
  }
});

const isFormValid = computed(() => {
  return (
    code.value.length >= 2 &&
    code.value.length <= 100 &&
    CODE_PATTERN.test(code.value) &&
    name.value.trim().length > 0 &&
    name.value.length <= 255 &&
    target.value.length > 0 &&
    isTargetValid.value &&
    queue.value.trim().length > 0 &&
    dispatchPoolId.value !== null &&
    selectedEventTypes.value.length > 0 &&
    maxAgeSeconds.value >= 1 &&
    timeoutSeconds.value >= 1
  );
});

// Filtered event types with CURRENT spec versions for selection
const eventTypeOptions = computed(() => {
  return eventTypes.value
    .filter(et => et.specVersions.some(sv => sv.status === 'CURRENT'))
    .map(et => ({
      id: et.id,
      code: et.code,
      name: et.name,
      currentVersion: et.specVersions.find(sv => sv.status === 'CURRENT')?.version || '',
    }));
});

onMounted(async () => {
  await Promise.all([loadDispatchPools(), loadEventTypes()]);
});

async function loadDispatchPools() {
  loadingPools.value = true;
  try {
    const response = await dispatchPoolsApi.list({ status: 'ACTIVE' });
    dispatchPools.value = response.pools;
  } catch (e) {
    console.error('Failed to load dispatch pools:', e);
  } finally {
    loadingPools.value = false;
  }
}

async function loadEventTypes() {
  loadingEventTypes.value = true;
  try {
    const response = await eventTypesApi.list({ status: 'CURRENT' });
    eventTypes.value = response.items;
  } catch (e) {
    console.error('Failed to load event types:', e);
  } finally {
    loadingEventTypes.value = false;
  }
}

function buildEventTypeBindings(): EventTypeBinding[] {
  return selectedEventTypes.value.map(etId => {
    const et = eventTypeOptions.value.find(e => e.id === etId);
    return {
      eventTypeId: etId,
      eventTypeCode: et?.code || '',
      specVersion: et?.currentVersion || '',
    };
  });
}

async function onSubmit() {
  if (!isFormValid.value) return;

  submitting.value = true;
  errorMessage.value = null;

  try {
    const subscription = await subscriptionsApi.create({
      code: code.value,
      name: name.value,
      description: description.value || undefined,
      target: target.value,
      queue: queue.value,
      eventTypes: buildEventTypeBindings(),
      dispatchPoolId: dispatchPoolId.value!,
      clientId: isAnchorLevel.value ? undefined : clientId.value || undefined,
      maxAgeSeconds: maxAgeSeconds.value,
      delaySeconds: delaySeconds.value,
      sequence: sequence.value,
      timeoutSeconds: timeoutSeconds.value,
      mode: mode.value,
      source: 'UI',
    });
    toast.add({ severity: 'success', summary: 'Success', detail: 'Subscription created', life: 3000 });
    router.push(`/subscriptions/${subscription.id}`);
  } catch (e) {
    errorMessage.value = e instanceof Error ? e.message : 'Failed to create subscription';
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Create Subscription</h1>
        <p class="page-subtitle">Configure a new event subscription for webhook delivery</p>
      </div>
    </header>

    <form @submit.prevent="onSubmit">
      <div class="form-card">
        <div class="form-section">
          <h3>Basic Information</h3>

          <div class="form-field">
            <label>Code <span class="required">*</span></label>
            <InputText
              v-model="code"
              placeholder="subscription-code"
              class="full-width"
              :invalid="!!(code && !isCodeValid)"
            />
            <small v-if="code && !isCodeValid" class="p-error">
              Lowercase letters, numbers, hyphens only. Must start with a letter.
            </small>
            <small v-else class="hint">
              Unique identifier for this subscription (2-100 characters)
            </small>
          </div>

          <div class="form-field">
            <label>Name <span class="required">*</span></label>
            <InputText
              v-model="name"
              placeholder="Subscription display name"
              class="full-width"
              :invalid="name.length > 255"
            />
            <small class="char-count">{{ name.length }} / 255</small>
          </div>

          <div class="form-field">
            <label>Description</label>
            <Textarea
              v-model="description"
              placeholder="Optional description..."
              class="full-width"
              rows="3"
            />
          </div>
        </div>

        <div class="form-section">
          <h3>Event Types</h3>

          <div class="form-field">
            <label>Event Types <span class="required">*</span></label>
            <MultiSelect
              v-model="selectedEventTypes"
              :options="eventTypeOptions"
              optionLabel="name"
              optionValue="id"
              placeholder="Select event types"
              class="full-width"
              :loading="loadingEventTypes"
              :disabled="loadingEventTypes"
              display="chip"
              filter
            >
              <template #option="{ option }">
                <div class="dropdown-option">
                  <span class="option-name">{{ option.name }}</span>
                  <span class="option-code">{{ option.code }} (v{{ option.currentVersion }})</span>
                </div>
              </template>
            </MultiSelect>
            <small class="hint">
              Select which event types this subscription will receive
            </small>
          </div>
        </div>

        <div class="form-section">
          <h3>Delivery Configuration</h3>

          <div class="form-field">
            <label>Target URL <span class="required">*</span></label>
            <InputText
              v-model="target"
              placeholder="https://example.com/webhook"
              class="full-width"
              :invalid="!!(target && !isTargetValid)"
            />
            <small v-if="target && !isTargetValid" class="p-error">
              Must be a valid HTTP or HTTPS URL
            </small>
            <small v-else class="hint">
              The webhook URL where events will be delivered
            </small>
          </div>

          <div class="form-field">
            <label>Queue <span class="required">*</span></label>
            <InputText
              v-model="queue"
              placeholder="default"
              class="full-width"
            />
            <small class="hint">
              Queue name for routing dispatch jobs
            </small>
          </div>

          <div class="form-field">
            <label>Dispatch Pool <span class="required">*</span></label>
            <Select
              v-model="dispatchPoolId"
              :options="dispatchPools"
              optionLabel="name"
              optionValue="id"
              placeholder="Select a dispatch pool"
              class="full-width"
              :loading="loadingPools"
              :disabled="loadingPools"
            >
              <template #option="{ option }">
                <div class="dropdown-option">
                  <span class="option-name">{{ option.name }}</span>
                  <span class="option-code">{{ option.code }} ({{ option.rateLimit }}/min, {{ option.concurrency }} concurrent)</span>
                </div>
              </template>
            </Select>
            <small class="hint">
              Rate-limiting pool for this subscription's dispatch jobs
            </small>
          </div>

          <div class="form-field">
            <label>Mode</label>
            <Select
              v-model="mode"
              :options="modeOptions"
              optionLabel="label"
              optionValue="value"
              class="full-width"
            />
            <small class="hint">
              How to handle dispatch failures
            </small>
          </div>
        </div>

        <div class="form-section">
          <h3>Timing</h3>

          <div class="form-row">
            <div class="form-field">
              <label>Max Age (seconds)</label>
              <InputNumber
                v-model="maxAgeSeconds"
                :min="1"
                class="full-width"
              />
              <small class="hint">Maximum age before events expire</small>
            </div>

            <div class="form-field">
              <label>Timeout (seconds)</label>
              <InputNumber
                v-model="timeoutSeconds"
                :min="1"
                class="full-width"
              />
              <small class="hint">Request timeout for webhook calls</small>
            </div>
          </div>

          <div class="form-row">
            <div class="form-field">
              <label>Delay (seconds)</label>
              <InputNumber
                v-model="delaySeconds"
                :min="0"
                class="full-width"
              />
              <small class="hint">Delay before dispatching</small>
            </div>

            <div class="form-field">
              <label>Sequence</label>
              <InputNumber
                v-model="sequence"
                :min="1"
                class="full-width"
              />
              <small class="hint">Processing order (lower = earlier)</small>
            </div>
          </div>
        </div>

        <div class="form-section">
          <h3>Scope</h3>

          <div class="form-field">
            <div class="checkbox-field">
              <Checkbox v-model="isAnchorLevel" :binary="true" inputId="anchorLevel" />
              <label for="anchorLevel">Anchor-level subscription (not client-scoped)</label>
            </div>
            <small class="hint">
              Anchor-level subscriptions receive events that are not scoped to a specific client.
            </small>
          </div>

          <div class="form-field" v-if="!isAnchorLevel">
            <label>Client</label>
            <ClientSelect
              v-model="clientId"
              placeholder="Search for a client (optional)"
            />
            <small class="hint">
              If specified, this subscription only receives events for this client.
            </small>
          </div>
        </div>

        <Message v-if="errorMessage" severity="error" class="error-message">
          {{ errorMessage }}
        </Message>

        <div class="form-actions">
          <Button
            label="Cancel"
            icon="pi pi-times"
            severity="secondary"
            outlined
            @click="router.push('/subscriptions')"
          />
          <Button
            label="Create Subscription"
            icon="pi pi-check"
            type="submit"
            :loading="submitting"
            :disabled="!isFormValid"
          />
        </div>
      </div>
    </form>
  </div>
</template>

<style scoped>
.page-container {
  max-width: 700px;
}

.form-card {
  background: white;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  padding: 24px;
}

.form-section {
  margin-bottom: 32px;
}

.form-section h3 {
  margin: 0 0 16px 0;
  font-size: 14px;
  font-weight: 600;
  color: #475569;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.form-field {
  margin-bottom: 20px;
}

.form-field > label {
  display: block;
  font-weight: 500;
  margin-bottom: 6px;
}

.form-field .required {
  color: #ef4444;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}

.full-width {
  width: 100%;
}

.char-count {
  display: block;
  text-align: right;
  font-size: 12px;
  color: #94a3b8;
  margin-top: 4px;
}

.hint {
  display: block;
  font-size: 12px;
  color: #64748b;
  margin-top: 4px;
}

.checkbox-field {
  display: flex;
  align-items: center;
  gap: 8px;
}

.checkbox-field label {
  margin: 0;
  cursor: pointer;
}

.dropdown-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.option-name {
  font-weight: 500;
}

.option-code {
  font-size: 12px;
  color: #64748b;
  font-family: monospace;
}

.error-message {
  margin-bottom: 16px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid #e2e8f0;
}

@media (max-width: 640px) {
  .form-row {
    grid-template-columns: 1fr;
  }
}
</style>
