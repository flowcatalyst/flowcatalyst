<script setup lang="ts">
import { toast } from "@/utils/errorBus";
import { computed, onMounted, ref } from "vue";
import { functionsApi } from "@/api/functions";
import { applicationsApi, type Application } from "@/api/applications";
import EntityDrawer from "@/components/drawer/EntityDrawer.vue";
import ClientSelect from "@/components/ClientSelect.vue";
import { useDrawerRoute } from "@/composables/useDrawerRoute";
import { isValidFunctionName, buildFunctionAddress } from "@/pages/functions/functionHelpers";

const emit = defineEmits<{
	changed: [];
}>();

const applications = ref<Application[]>([]);

// Form fields
const applicationId = ref<string | null>(null);
const name = ref("");
const clientId = ref<string | null>(null);
const pool = ref("");
const warm = ref(false);
const memoryMb = ref<number | null>(null);
const maxConcurrency = ref<number | null>(null);
const timeoutMs = ref<number | null>(null);
const maxBodyBytes = ref<number | null>(null);

const selectedApplication = computed(
	() => applications.value.find((a) => a.id === applicationId.value) ?? null,
);

const addressPreview = computed(() =>
	buildFunctionAddress(selectedApplication.value?.code, name.value),
);

const isNameValid = computed(() => !name.value || isValidFunctionName(name.value));

const isFormValid = computed(
	() => !!applicationId.value && name.value.length > 0 && isValidFunctionName(name.value),
);

// Cheap dirty check: anything typed or selected counts.
const dirty = computed(
	() =>
		applicationId.value !== null ||
		name.value !== "" ||
		clientId.value !== null ||
		pool.value !== "" ||
		warm.value !== false,
);

const drawer = ref<InstanceType<typeof EntityDrawer> | null>(null);
const { goToList, replaceToDetail } = useDrawerRoute({
	listPath: "/functions",
	dirty,
});

const submitting = ref(false);
const errorMessage = ref<string | null>(null);

onMounted(async () => {
	try {
		const response = await applicationsApi.list();
		applications.value = response.applications || [];
	} catch (e) {
		console.error("Failed to load application list", e);
	}
});

async function onSubmit() {
	if (!isFormValid.value || !applicationId.value) return;

	submitting.value = true;
	errorMessage.value = null;

	try {
		const created = await functionsApi.create({
			applicationId: applicationId.value,
			name: name.value,
			clientId: clientId.value || undefined,
			pool: pool.value || undefined,
			warm: warm.value,
			limits: {
				memoryMb: memoryMb.value ?? undefined,
				maxConcurrency: maxConcurrency.value ?? undefined,
				timeoutMs: timeoutMs.value ?? undefined,
				maxBodyBytes: maxBodyBytes.value ?? undefined,
			},
		});
		toast.success("Success", "Function created");
		emit("changed");
		replaceToDetail(created.id);
	} catch (e) {
		errorMessage.value = e instanceof Error ? e.message : "Failed to create function";
	} finally {
		submitting.value = false;
	}
}
</script>

<template>
  <EntityDrawer
    ref="drawer"
    title="Create Function"
    subtitle="Register a new function; publish and promote a version separately"
    :dirty="dirty"
    @close="goToList()"
  >
    <div class="form-section">
      <h3>Identity</h3>

      <div class="form-field">
        <label>Application <span class="required">*</span></label>
        <Select
          v-model="applicationId"
          :options="applications"
          optionLabel="name"
          optionValue="id"
          placeholder="Select an application"
          filter
          class="full-width"
        >
          <template #option="{ option }">
            <span>{{ option.name }}</span>
            <code class="app-code-inline">{{ option.code }}</code>
          </template>
        </Select>
        <small class="hint">The function's owning application; its code prefixes the address.</small>
      </div>

      <div class="form-field">
        <label>Name <span class="required">*</span></label>
        <InputText
          v-model="name"
          placeholder="invoice-pdf"
          class="full-width"
          :invalid="!!(name && !isNameValid)"
        />
        <small v-if="name && !isNameValid" class="p-error">
          Lowercase letters, numbers, hyphens only. Must start with a letter (max 63 characters).
        </small>
        <small v-else class="hint">Becomes part of the address, which never changes.</small>
      </div>

      <div v-if="addressPreview" class="address-preview">
        <span class="address-preview-label">Address</span>
        <code>{{ addressPreview }}</code>
      </div>

      <div class="form-field">
        <label>Client</label>
        <ClientSelect v-model="clientId" placeholder="Search for a client (optional)" />
        <small class="hint">
          Optional. Leave blank for a platform-owned function.
        </small>
      </div>
    </div>

    <div class="form-section">
      <h3>Runner</h3>

      <div class="form-field">
        <label>Runner Pool</label>
        <InputText v-model="pool" placeholder="default" class="full-width" />
        <small class="hint">The runner pool that hosts the function. Left blank, it runs in the default pool.</small>
      </div>

      <div class="form-field">
        <div class="checkbox-field">
          <Checkbox v-model="warm" :binary="true" inputId="warm" />
          <label for="warm">Keep warm</label>
        </div>
        <small class="hint">
          A warm function keeps one pre-instantiated instance ready at all times.
        </small>
      </div>
    </div>

    <div class="form-section">
      <h3>Limits</h3>

      <div class="form-row">
        <div class="form-field">
          <label>Memory (MB)</label>
          <InputNumber v-model="memoryMb" :min="1" class="full-width" placeholder="64" />
        </div>
        <div class="form-field">
          <label>Max Concurrency</label>
          <InputNumber v-model="maxConcurrency" :min="1" class="full-width" placeholder="16" />
        </div>
      </div>
      <div class="form-row">
        <div class="form-field">
          <label>Timeout (ms)</label>
          <InputNumber v-model="timeoutMs" :min="1" class="full-width" placeholder="30000" />
        </div>
        <div class="form-field">
          <label>Max Body Bytes</label>
          <InputNumber v-model="maxBodyBytes" :min="1" class="full-width" placeholder="1048576" />
        </div>
      </div>
      <small class="hint">Optional. Left blank, defaults apply (shown as placeholders above).</small>
    </div>

    <Message v-if="errorMessage" severity="error" class="error-message">
      {{ errorMessage }}
    </Message>

    <template #footer>
      <FcFormActions :bordered="false">
        <Button
          label="Cancel"
          icon="pi pi-times"
          severity="secondary"
          outlined
          :disabled="submitting"
          @click="drawer?.close()"
        />
        <Button
          label="Create Function"
          icon="pi pi-check"
          :loading="submitting"
          :disabled="!isFormValid"
          @click="onSubmit"
        />
      </FcFormActions>
    </template>
  </EntityDrawer>
</template>

<style scoped>
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

.app-code-inline {
  margin-left: 8px;
  font-size: 12px;
  color: #64748b;
}

.address-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: -8px 0 20px;
  padding: 8px 12px;
  background: #f1f5f9;
  border-radius: 6px;
  font-size: 13px;
}

.address-preview-label {
  color: #64748b;
}

.error-message {
  margin-bottom: 16px;
}

@media (max-width: 640px) {
  .form-row {
    grid-template-columns: 1fr;
  }
}
</style>
