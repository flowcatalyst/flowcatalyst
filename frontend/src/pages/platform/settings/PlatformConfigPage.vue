<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useConfirm } from "primevue/useconfirm";
import { applicationsApi } from "@/api/applications";
import { configApi, type PlatformConfig } from "@/api/config";
import { useAuthStore } from "@/stores/auth";
import { userHasPermission } from "@/stores/permissions";
import { toast } from "@/utils/errorBus";
import { getErrorMessage } from "@/utils/errors";

const authStore = useAuthStore();
const confirm = useConfirm();

const canEdit = computed(() =>
	userHasPermission(authStore.user, "platform:admin:config:update"),
);
const canReveal = computed(() =>
	userHasPermission(authStore.user, "platform:admin:config:reveal"),
);

const apps = ref<{ label: string; value: string }[]>([]);
const appCode = ref("");
const rows = ref<PlatformConfig[]>([]);
const loading = ref(false);
const error = ref("");
const search = ref("");

const filtered = computed(() => {
	const q = search.value.trim().toLowerCase();
	if (!q) return rows.value;
	return rows.value.filter((r) =>
		`${r.section} ${r.property} ${r.description ?? ""}`.toLowerCase().includes(q),
	);
});

onMounted(async () => {
	try {
		const res = await applicationsApi.list({ activeOnly: true });
		const codes = new Set(["platform", ...res.applications.map((a) => a.code)]);
		apps.value = [...codes].sort().map((c) => ({ label: c, value: c }));
		appCode.value = "platform";
	} catch (e) {
		error.value = getErrorMessage(e, "Could not load applications.");
	}
});

watch(appCode, load);

async function load() {
	if (!appCode.value) return;
	loading.value = true;
	error.value = "";
	try {
		rows.value = (await configApi.listProperties(appCode.value)).items;
	} catch (e) {
		rows.value = [];
		error.value = getErrorMessage(e, "Could not load the configuration.");
	} finally {
		loading.value = false;
	}
}

// ── edit dialog ─────────────────────────────────────────────────────────
const typeOptions = [
	{ label: "Plain text", value: "PLAIN" },
	{ label: "Secret", value: "SECRET" },
];
const dialog = ref(false);
const editing = ref<PlatformConfig | null>(null);
const form = ref({
	section: "",
	property: "",
	valueType: "PLAIN" as "PLAIN" | "SECRET",
	value: "",
	description: "",
	clientId: "",
});
const saving = ref(false);
const formError = ref("");

function openNew() {
	editing.value = null;
	form.value = { section: "", property: "", valueType: "PLAIN", value: "", description: "", clientId: "" };
	formError.value = "";
	dialog.value = true;
}

function openEdit(row: PlatformConfig) {
	editing.value = row;
	form.value = {
		section: row.section,
		property: row.property,
		valueType: row.valueType as "PLAIN" | "SECRET",
		// A secret's value is write-only: never prefilled.
		value: row.valueType === "SECRET" ? "" : row.value,
		description: row.description ?? "",
		clientId: row.clientId ?? "",
	};
	formError.value = "";
	dialog.value = true;
}

async function save() {
	const f = form.value;
	if (!f.section.trim() || !f.property.trim()) {
		formError.value = "Section and property are required.";
		return;
	}
	if (f.value === "") {
		formError.value = f.valueType === "SECRET" ? "Enter the secret value." : "A value is required.";
		return;
	}
	saving.value = true;
	formError.value = "";
	try {
		await configApi.setConfig(
			appCode.value,
			f.section.trim(),
			f.property.trim(),
			{
				value: f.value,
				valueType: f.valueType,
				description: f.description.trim() || undefined,
			},
			f.clientId.trim() || undefined,
		);
		dialog.value = false;
		toast.success("Saved", `${f.section.trim()}/${f.property.trim()} saved.`);
		await load();
	} catch (e) {
		formError.value = getErrorMessage(e, "Could not save.");
	} finally {
		saving.value = false;
	}
}

function remove(row: PlatformConfig) {
	confirm.require({
		header: "Delete property",
		message: `Delete ${row.section}/${row.property}${row.clientId ? ` for client ${row.clientId}` : ""}? Applications reading it will stop finding it.`,
		icon: "pi pi-trash",
		acceptProps: { label: "Delete", severity: "danger" },
		rejectProps: { label: "Cancel", text: true },
		accept: async () => {
			try {
				await configApi.deleteConfig(appCode.value, row.section, row.property, row.clientId ?? undefined);
				toast.success("Deleted", `${row.section}/${row.property} deleted.`);
				await load();
			} catch (e) {
				toast.error("Delete failed", getErrorMessage(e, "Could not delete."));
			}
		},
	});
}

// ── reveal ──────────────────────────────────────────────────────────────
const revealDialog = ref(false);
const revealTarget = ref<PlatformConfig | null>(null);
const revealed = ref("");
const revealing = ref(false);

function askReveal(row: PlatformConfig) {
	confirm.require({
		header: "Reveal secret",
		message: `Show the value of ${row.section}/${row.property}? This is recorded in the audit log under your name.`,
		icon: "pi pi-eye",
		acceptProps: { label: "Reveal" },
		rejectProps: { label: "Cancel", text: true },
		accept: async () => {
			revealing.value = true;
			try {
				const res = await configApi.revealSecret(appCode.value, row.section, row.property, row.clientId ?? undefined);
				revealTarget.value = row;
				revealed.value = res.value;
				revealDialog.value = true;
			} catch (e) {
				toast.error("Reveal failed", getErrorMessage(e, "Could not reveal the secret."));
			} finally {
				revealing.value = false;
			}
		},
	});
}

function closeReveal() {
	revealDialog.value = false;
	revealed.value = ""; // do not keep the plaintext around
}

async function copyRevealed() {
	await navigator.clipboard.writeText(revealed.value);
	toast.success("Copied", "Secret copied to the clipboard.");
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Configuration</h1>
        <p class="page-subtitle">
          Platform configuration per application. Secret values are write-only here and are never
          shown unless you reveal one, which is audited.
        </p>
      </div>
      <Button v-if="canEdit" label="Add property" icon="pi pi-plus" :disabled="!appCode" @click="openNew" />
    </header>

    <Message v-if="error" severity="error" class="error-message">{{ error }}</Message>

    <div class="fc-card">
      <div class="toolbar">
        <Select
          v-model="appCode"
          :options="apps"
          option-label="label"
          option-value="value"
          placeholder="Application"
          class="app-select"
          filter
        />
        <IconField class="search-wrapper">
          <InputIcon class="pi pi-search" />
          <InputText v-model="search" placeholder="Search section, property, description..." />
        </IconField>
      </div>

      <div v-if="loading" class="loading-container"><ProgressSpinner strokeWidth="3" /></div>

      <DataTable v-else :value="filtered" stripedRows paginator :rows="25" emptyMessage="No properties for this application">
        <Column field="section" header="Section" sortable />
        <Column field="property" header="Property" sortable />
        <Column header="Scope" style="width: 9rem">
          <template #body="{ data }">
            <Tag v-if="data.clientId" :value="data.clientId" severity="info" />
            <span v-else class="muted">Global</span>
          </template>
        </Column>
        <Column header="Value">
          <template #body="{ data }">
            <span v-if="data.valueType === 'SECRET'" class="secret-value"><i class="pi pi-lock" /> ••••••••</span>
            <code v-else class="value-code">{{ data.value }}</code>
          </template>
        </Column>
        <Column field="description" header="Description">
          <template #body="{ data }"><span class="muted">{{ data.description || '—' }}</span></template>
        </Column>
        <Column header="" style="width: 9rem">
          <template #body="{ data }">
            <Button
              v-if="data.valueType === 'SECRET' && canReveal"
              icon="pi pi-eye"
              text
              rounded
              v-tooltip="'Reveal (audited)'"
              :loading="revealing"
              @click="askReveal(data)"
            />
            <Button v-if="canEdit" icon="pi pi-pencil" text rounded v-tooltip="'Edit'" @click="openEdit(data)" />
            <Button v-if="canEdit" icon="pi pi-trash" text rounded severity="danger" v-tooltip="'Delete'" @click="remove(data)" />
          </template>
        </Column>
      </DataTable>
    </div>

    <Dialog v-model:visible="dialog" :header="editing ? 'Edit property' : 'Add property'" modal :style="{ width: '34rem', maxWidth: 'calc(100vw - 2rem)' }">
      <div class="dialog-content">
        <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
        <div class="field-row">
          <div class="field">
            <label for="cfg-section">Section *</label>
            <InputText id="cfg-section" v-model="form.section" :disabled="!!editing" class="w-full" />
          </div>
          <div class="field">
            <label for="cfg-property">Property *</label>
            <InputText id="cfg-property" v-model="form.property" :disabled="!!editing" class="w-full" />
          </div>
        </div>
        <div class="field">
          <label for="cfg-client">Client (optional)</label>
          <InputText id="cfg-client" v-model="form.clientId" :disabled="!!editing" placeholder="clt_… — empty for a platform-wide value" class="w-full" />
        </div>
        <div class="field">
          <label>Type</label>
          <SelectButton v-model="form.valueType" :options="typeOptions" option-label="label" option-value="value" :allow-empty="false" />
        </div>
        <div class="field">
          <label for="cfg-value">{{ form.valueType === 'SECRET' ? 'Secret value *' : 'Value *' }}</label>
          <Password v-if="form.valueType === 'SECRET'" v-model="form.value" input-id="cfg-value" :feedback="false" toggle-mask fluid :placeholder="editing ? 'Enter a new value to replace the secret' : ''" autocomplete="off" />
          <Textarea v-else id="cfg-value" v-model="form.value" rows="3" class="w-full" />
          <small v-if="form.valueType === 'SECRET'" class="field-help">Stored encrypted. It cannot be read back here; only a reveal (audited) returns it.</small>
        </div>
        <div class="field">
          <label for="cfg-desc">Description</label>
          <InputText id="cfg-desc" v-model="form.description" class="w-full" />
        </div>
      </div>
      <template #footer>
        <Button label="Cancel" text :disabled="saving" @click="dialog = false" />
        <Button label="Save" icon="pi pi-check" :loading="saving" @click="save" />
      </template>
    </Dialog>

    <Dialog v-model:visible="revealDialog" header="Secret value" modal :style="{ width: '34rem', maxWidth: 'calc(100vw - 2rem)' }" @hide="closeReveal">
      <div class="dialog-content">
        <p class="muted">{{ revealTarget?.section }}/{{ revealTarget?.property }} — this reveal was recorded in the audit log.</p>
        <code class="value-code reveal-code">{{ revealed }}</code>
      </div>
      <template #footer>
        <Button label="Copy" icon="pi pi-copy" text @click="copyRevealed" />
        <Button label="Close" @click="closeReveal" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.app-select {
  min-width: 14rem;
}
.search-wrapper {
  flex: 1;
  min-width: 14rem;
}
.muted {
  color: var(--p-text-muted-color);
}
.secret-value {
  color: var(--p-text-muted-color);
  letter-spacing: 0.1em;
}
.value-code {
  font-family: var(--font-mono, monospace);
  word-break: break-all;
}
.reveal-code {
  display: block;
  padding: 12px;
  background: var(--p-content-hover-background);
  border-radius: 6px;
  user-select: all;
}
.field-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.field {
  margin-bottom: 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.field-help {
  color: var(--p-text-muted-color);
}
</style>
