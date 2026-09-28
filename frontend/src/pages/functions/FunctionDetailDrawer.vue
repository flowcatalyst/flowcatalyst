<script setup lang="ts">
import { toast } from "@/utils/errorBus";
import { computed, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useConfirm } from "primevue/useconfirm";
import {
	functionsApi,
	type FunctionAlias,
	type FunctionRecord,
	type FunctionSetting,
	type FunctionSettingKind,
	type FunctionVersion,
} from "@/api/functions";
import { useAuthStore } from "@/stores/auth";
import { userHasPermission } from "@/stores/permissions";
import { useClientOptions } from "@/composables/useClientOptions";
import EntityDrawer from "@/components/drawer/EntityDrawer.vue";
import { useDrawerRoute } from "@/composables/useDrawerRoute";
import { useDirtyForm } from "@/composables/useDirtyForm";
import {
	aliasesForVersion,
	computeMissingSettings,
	describeFailure,
	describeWiring,
	hasMissingSettings,
	liveOrNewestVersion,
	parseDescribe,
	versionDigestShort,
	type FunctionDescribe,
} from "@/pages/functions/functionHelpers";

const emit = defineEmits<{
	changed: [];
}>();

const route = useRoute();
const confirm = useConfirm();
const authStore = useAuthStore();
const { getLabel: getClientLabel, ensureLoaded: ensureClients } = useClientOptions();

const canManage = computed(() =>
	userHasPermission(authStore.user, "platform:function:function:manage"),
);
const canPromote = computed(() =>
	userHasPermission(authStore.user, "platform:function:alias:promote"),
);
const canManageSecrets = computed(() =>
	userHasPermission(authStore.user, "platform:function:secret:manage"),
);

const editing = ref(false);

// Edit form
const editDescription = ref("");
const editPool = ref("");
const editWarm = ref(false);
const editMemoryMb = ref<number | null>(null);
const editMaxConcurrency = ref<number | null>(null);
const editTimeoutMs = ref<number | null>(null);
const editMaxBodyBytes = ref<number | null>(null);

const { dirty, markClean, reset: resetDirty } = useDirtyForm(() => ({
	description: editDescription.value,
	pool: editPool.value,
	warm: editWarm.value,
	memoryMb: editMemoryMb.value,
	maxConcurrency: editMaxConcurrency.value,
	timeoutMs: editTimeoutMs.value,
	maxBodyBytes: editMaxBodyBytes.value,
}));

const drawer = ref<InstanceType<typeof EntityDrawer> | null>(null);
const { id, goToList } = useDrawerRoute({
	listPath: "/functions",
	dirty: computed(() => editing.value && dirty.value),
});

const loading = ref(true);
const loadError = ref<string | null>(null);
const fn = ref<FunctionRecord | null>(null);
const versions = ref<FunctionVersion[]>([]);
const aliases = ref<FunctionAlias[]>([]);
const settings = ref<FunctionSetting[]>([]);
const saving = ref(false);

// Reactive param: the drawer instance is reused when switching between rows.
watch(
	id,
	async (value) => {
		if (!value) return;
		editing.value = false;
		resetDirty();
		await loadAll(value);
		if (route.query["edit"] === "true") {
			startEditing();
		}
	},
	{ immediate: true },
);

async function loadAll(functionId: string) {
	loading.value = true;
	loadError.value = null;
	try {
		const [record, versionList, aliasList, settingList] = await Promise.all([
			functionsApi.get(functionId),
			functionsApi.listVersions(functionId),
			functionsApi.listAliases(functionId),
			functionsApi.listSettings(functionId),
			ensureClients(),
		]);
		fn.value = record;
		versions.value = versionList.toSorted((a, b) => b.number - a.number);
		aliases.value = aliasList;
		settings.value = settingList;
	} catch {
		fn.value = null;
		loadError.value = "Function not found";
	} finally {
		loading.value = false;
	}
}

async function reloadVersionsAndAliases() {
	if (!fn.value) return;
	const [versionList, aliasList] = await Promise.all([
		functionsApi.listVersions(fn.value.id),
		functionsApi.listAliases(fn.value.id),
	]);
	versions.value = versionList.toSorted((a, b) => b.number - a.number);
	aliases.value = aliasList;
}

async function reloadSettings() {
	if (!fn.value) return;
	settings.value = await functionsApi.listSettings(fn.value.id);
}

// --- header / details ---

const clientLabel = computed(() => {
	if (!fn.value?.clientId) return "Platform";
	return getClientLabel(fn.value.clientId) || fn.value.clientId;
});

const poolLabel = computed(() => fn.value?.pool || "default");

function startEditing() {
	if (!fn.value) return;
	editDescription.value = fn.value.description || "";
	editPool.value = fn.value.pool || "";
	editWarm.value = fn.value.warm;
	editMemoryMb.value = fn.value.limits?.memoryMb ?? null;
	editMaxConcurrency.value = fn.value.limits?.maxConcurrency ?? null;
	editTimeoutMs.value = fn.value.limits?.timeoutMs ?? null;
	editMaxBodyBytes.value = fn.value.limits?.maxBodyBytes ?? null;
	editing.value = true;
	markClean();
}

function cancelEditing() {
	editing.value = false;
	resetDirty();
}

async function saveChanges() {
	if (!fn.value) return;
	saving.value = true;
	const functionId = fn.value.id;
	try {
		await functionsApi.update(functionId, {
			description: editDescription.value || undefined,
			pool: editPool.value || undefined,
			clearPool: !editPool.value && !!fn.value.pool,
			warm: editWarm.value,
			limits: {
				memoryMb: editMemoryMb.value ?? undefined,
				maxConcurrency: editMaxConcurrency.value ?? undefined,
				timeoutMs: editTimeoutMs.value ?? undefined,
				maxBodyBytes: editMaxBodyBytes.value ?? undefined,
			},
		});
		fn.value = await functionsApi.get(functionId);
		editing.value = false;
		resetDirty();
		toast.success("Success", "Function updated");
		emit("changed");
	} catch {
		// update errors surface via the global error toast
	} finally {
		saving.value = false;
	}
}

function confirmDelete() {
	confirm.require({
		message:
			"Delete this function? This permanently removes it, along with every version, alias, wiring (subscriptions and scheduled jobs) and artifact. Cannot be undone.",
		header: "Delete Function",
		icon: "pi pi-exclamation-triangle",
		acceptLabel: "Delete",
		acceptClass: "p-button-danger",
		accept: deleteFunction,
	});
}

async function deleteFunction() {
	if (!fn.value) return;
	try {
		await functionsApi.delete(fn.value.id);
		toast.success("Success", "Function deleted");
		emit("changed");
		editing.value = false;
		void drawer.value?.close(true);
	} catch {
		// errors surface via the global error toast
	}
}

// --- versions ---

function statusSeverity(status: string) {
	switch (status) {
		case "PUBLISHED":
			return "info";
		case "READY":
			return "success";
		case "FAILED":
			return "danger";
		case "RETIRED":
			return "secondary";
		default:
			return "secondary";
	}
}

function versionAliasNames(v: FunctionVersion): string[] {
	return aliasesForVersion(aliases.value, v.number);
}

async function promoteToLive(v: FunctionVersion) {
	if (!fn.value) return;
	try {
		const result = await functionsApi.putAlias(fn.value.id, "live", v.number);
		await reloadVersionsAndAliases();
		const wiringText = describeWiring(result.wiring);
		toast.success(
			"Promoted",
			`Version ${v.number} is now live${wiringText ? ` — ${wiringText}` : ""}`,
		);
	} catch {
		// errors surface via the global error toast
	}
}

function confirmRetire(v: FunctionVersion) {
	confirm.require({
		message: `Retire version ${v.number}? A version still pointed at by an alias cannot be retired.`,
		header: "Retire Version",
		icon: "pi pi-exclamation-triangle",
		acceptLabel: "Retire",
		acceptClass: "p-button-warning",
		accept: () => retireVersion(v),
	});
}

async function retireVersion(v: FunctionVersion) {
	if (!fn.value) return;
	try {
		await functionsApi.retireVersion(fn.value.id, v.number);
		await reloadVersionsAndAliases();
		toast.success("Success", `Version ${v.number} retired`);
	} catch {
		// errors surface via the global error toast (surfaces RETIRE_IN_USE etc.)
	}
}

// --- aliases / point-alias dialog ---

const pointAliasDialogVisible = ref(false);
const pointAliasName = ref("");
const pointAliasVersion = ref<number | null>(null);
const pointAliasSubmitting = ref(false);

const readyVersionOptions = computed(() =>
	versions.value
		.filter((v) => v.status === "READY")
		.map((v) => ({ label: `Version ${v.number}`, value: v.number })),
);

function openPointAliasDialog(version?: FunctionVersion) {
	pointAliasName.value = version ? "live" : "";
	pointAliasVersion.value = version ? version.number : null;
	pointAliasDialogVisible.value = true;
}

async function submitPointAlias() {
	if (!fn.value || !pointAliasName.value.trim() || pointAliasVersion.value == null) return;
	pointAliasSubmitting.value = true;
	try {
		const result = await functionsApi.putAlias(
			fn.value.id,
			pointAliasName.value.trim(),
			pointAliasVersion.value,
		);
		await reloadVersionsAndAliases();
		const wiringText = describeWiring(result.wiring);
		toast.success(
			"Alias updated",
			`${pointAliasName.value.trim()} → version ${pointAliasVersion.value}${wiringText ? ` — ${wiringText}` : ""}`,
		);
		pointAliasDialogVisible.value = false;
	} catch {
		// errors surface via the global error toast
	} finally {
		pointAliasSubmitting.value = false;
	}
}

function confirmRemoveAlias(alias: FunctionAlias) {
	const liveWarning =
		alias.name === "live"
			? " This is the live alias — removing it unwires this function's subscriptions and schedules."
			: "";
	confirm.require({
		message: `Remove alias "${alias.name}"?${liveWarning}`,
		header: "Remove Alias",
		icon: "pi pi-exclamation-triangle",
		acceptLabel: "Remove",
		acceptClass: "p-button-danger",
		accept: () => removeAlias(alias),
	});
}

async function removeAlias(alias: FunctionAlias) {
	if (!fn.value) return;
	try {
		const wiring = await functionsApi.deleteAlias(fn.value.id, alias.name);
		await reloadVersionsAndAliases();
		const wiringText = describeWiring(wiring);
		toast.success("Success", `Alias removed${wiringText ? ` — ${wiringText}` : ""}`);
	} catch {
		// errors surface via the global error toast
	}
}

// --- what it declares ---

const describeSource = computed(() => liveOrNewestVersion(versions.value, aliases.value));
const describeIsLive = computed(
	() =>
		!!describeSource.value &&
		aliases.value.some((a) => a.name === "live" && a.version === describeSource.value?.number),
);
const declared = computed<FunctionDescribe>(() => parseDescribe(describeSource.value?.describe));

function authSeverity(auth: string) {
	switch (auth) {
		case "webhook":
			return "info";
		case "platform":
			return "success";
		case "none":
			return "secondary";
		default:
			return "secondary";
	}
}

// --- settings ---

const missing = computed(() => computeMissingSettings(declared.value, settings.value));
const anyMissing = computed(() => hasMissingSettings(missing.value));

interface SettingRow {
	key: string;
	kind: FunctionSettingKind;
	value?: string;
	set: boolean;
}

function buildRows(kind: FunctionSettingKind, declaredKeys: string[]): SettingRow[] {
	const byKey = new Map(settings.value.filter((s) => s.kind === kind).map((s) => [s.key, s]));
	const keys = new Set<string>([...declaredKeys, ...byKey.keys()]);
	return [...keys].toSorted().map((key) => {
		const row = byKey.get(key);
		return { key, kind, value: row?.value, set: !!row };
	});
}

const configRows = computed(() => buildRows("CONFIG", declared.value.config));
const secretRows = computed(() => buildRows("SECRET", declared.value.secrets));
const dbRows = computed(() => buildRows("DB", declared.value.db));

const settingDialogVisible = ref(false);
const settingDialogKind = ref<FunctionSettingKind>("CONFIG");
const settingDialogKey = ref("");
const settingDialogKeyEditable = ref(false);
const settingDialogValue = ref("");
const settingDialogSubmitting = ref(false);

function openSettingDialog(kind: FunctionSettingKind, key = "", currentValue?: string) {
	settingDialogKind.value = kind;
	settingDialogKey.value = key;
	settingDialogKeyEditable.value = key === "";
	// Config values are plain text and safe to prefill for editing; secrets
	// and DB DSNs are write-only and never come back from the API, so the
	// field always starts blank for them.
	settingDialogValue.value = kind === "CONFIG" ? (currentValue ?? "") : "";
	settingDialogVisible.value = true;
}

function settingKindLabel(kind: FunctionSettingKind): string {
	switch (kind) {
		case "CONFIG":
			return "config value";
		case "SECRET":
			return "secret";
		case "DB":
			return "database binding";
	}
}

async function submitSettingDialog() {
	if (!fn.value || !settingDialogKey.value.trim()) return;
	settingDialogSubmitting.value = true;
	const functionId = fn.value.id;
	const key = settingDialogKey.value.trim();
	const value = settingDialogValue.value;
	try {
		if (settingDialogKind.value === "CONFIG") {
			await functionsApi.putConfig(functionId, key, value);
		} else if (settingDialogKind.value === "SECRET") {
			await functionsApi.putSecret(functionId, key, value);
		} else {
			await functionsApi.putDb(functionId, key, value);
		}
		await reloadSettings();
		toast.success("Success", `Set ${settingKindLabel(settingDialogKind.value)} "${key}"`);
		settingDialogVisible.value = false;
	} catch {
		// errors surface via the global error toast
	} finally {
		settingDialogSubmitting.value = false;
	}
}

function confirmRemoveSetting(row: SettingRow) {
	confirm.require({
		message: `Remove ${settingKindLabel(row.kind)} "${row.key}"?`,
		header: "Remove Setting",
		icon: "pi pi-exclamation-triangle",
		acceptLabel: "Remove",
		acceptClass: "p-button-danger",
		accept: () => removeSetting(row),
	});
}

async function removeSetting(row: SettingRow) {
	if (!fn.value) return;
	const functionId = fn.value.id;
	try {
		if (row.kind === "CONFIG") {
			await functionsApi.deleteConfig(functionId, row.key);
		} else if (row.kind === "SECRET") {
			await functionsApi.deleteSecret(functionId, row.key);
		} else {
			await functionsApi.deleteDb(functionId, row.key);
		}
		await reloadSettings();
		toast.success("Success", `Removed ${settingKindLabel(row.kind)} "${row.key}"`);
	} catch {
		// errors surface via the global error toast
	}
}

// --- formatting ---

function formatDate(dateString: string | undefined | null) {
	if (!dateString) return "—";
	return new Date(dateString).toLocaleString();
}
</script>

<template>
  <EntityDrawer
    ref="drawer"
    size="wide"
    :title="fn?.address || 'Function'"
    :subtitle="fn ? `${fn.applicationCode} · ${clientLabel}` : undefined"
    :loading="loading"
    :error="loadError"
    :dirty="editing && dirty"
    @close="goToList()"
  >
    <template v-if="fn" #header-extra>
      <Tag :value="poolLabel" severity="info" />
      <Tag :value="fn.warm ? 'Warm' : 'Lazy'" :severity="fn.warm ? 'success' : 'secondary'" />
    </template>

    <template v-if="fn">
      <!-- Details -->
      <FcFormSection title="Function Details" flat>
        <template v-if="!editing && canManage" #actions>
          <Button icon="pi pi-pencil" label="Edit" text @click="startEditing" />
        </template>

        <template v-if="editing">
          <div class="fc-form-grid">
            <FcFormField label="Description" span>
              <template #default="{ id: fieldId }">
                <Textarea :id="fieldId" v-model="editDescription" rows="2" />
              </template>
            </FcFormField>
            <FcFormField label="Runner Pool" help="The runner pool that hosts the function. Blank runs it in the default pool.">
              <template #default="{ id: fieldId }">
                <InputText :id="fieldId" v-model="editPool" placeholder="default" />
              </template>
            </FcFormField>
            <FcFormField label="Warm">
              <template #default="{ id: fieldId }">
                <div class="warm-toggle">
                  <ToggleSwitch :inputId="fieldId" v-model="editWarm" />
                  <label :for="fieldId">Keep one instance pre-instantiated</label>
                </div>
              </template>
            </FcFormField>
            <FcFormField label="Memory (MB)">
              <template #default="{ id: fieldId }">
                <InputNumber :inputId="fieldId" v-model="editMemoryMb" :min="1" placeholder="64" />
              </template>
            </FcFormField>
            <FcFormField label="Max Concurrency">
              <template #default="{ id: fieldId }">
                <InputNumber :inputId="fieldId" v-model="editMaxConcurrency" :min="1" placeholder="16" />
              </template>
            </FcFormField>
            <FcFormField label="Timeout (ms)">
              <template #default="{ id: fieldId }">
                <InputNumber :inputId="fieldId" v-model="editTimeoutMs" :min="1" placeholder="30000" />
              </template>
            </FcFormField>
            <FcFormField label="Max Body Bytes">
              <template #default="{ id: fieldId }">
                <InputNumber :inputId="fieldId" v-model="editMaxBodyBytes" :min="1" placeholder="1048576" />
              </template>
            </FcFormField>
          </div>
        </template>

        <template v-else>
          <div class="fc-detail-grid">
            <FcDetailField label="Address">
              <code>{{ fn.address }}</code>
            </FcDetailField>
            <FcDetailField label="Application" :value="`${fn.applicationCode}`" />
            <FcDetailField label="Client" :value="clientLabel" />
            <FcDetailField label="Runner Pool" :value="poolLabel" />
            <FcDetailField label="Warm" :value="fn.warm ? 'Yes' : 'No'" />
            <FcDetailField v-if="fn.description" label="Description" :value="fn.description" span />
            <FcDetailField label="Memory" :value="`${fn.limits?.memoryMb ?? 64} MB`" />
            <FcDetailField label="Max Concurrency" :value="fn.limits?.maxConcurrency ?? 16" />
            <FcDetailField label="Timeout" :value="`${fn.limits?.timeoutMs ?? 30000} ms`" />
            <FcDetailField label="Max Body" :value="`${fn.limits?.maxBodyBytes ?? 1048576} bytes`" />
            <FcDetailField label="Created" :value="formatDate(fn.createdAt)" />
            <FcDetailField label="Updated" :value="formatDate(fn.updatedAt)" />
          </div>
        </template>
      </FcFormSection>

      <!-- Versions -->
      <FcFormSection
        title="Versions"
        :description="`Publish and deploy from the CLI: fcdev fn deploy ${fn.address} .`"
        flat
      >
        <DataTable :value="versions" size="small">
          <Column field="number" header="#" style="width: 48px" />
          <Column field="runtime" header="Runtime" style="width: 80px">
            <template #body="{ data }">{{ data.runtime || "wasm" }}</template>
          </Column>
          <Column header="Status">
            <template #body="{ data }">
              <Tag :value="data.status" :severity="statusSeverity(data.status)" />
              <div v-if="data.status === 'FAILED'" class="failure-reason">
                {{ describeFailure(data.failure) || "Load failed" }}
              </div>
            </template>
          </Column>
          <Column header="Digest">
            <template #body="{ data }">
              <code class="digest">{{ versionDigestShort(data.digest) }}</code>
            </template>
          </Column>
          <Column header="Published">
            <template #body="{ data }">{{ formatDate(data.readyAt || data.createdAt) }}</template>
          </Column>
          <Column header="Aliases">
            <template #body="{ data }">
              <div class="alias-chips">
                <Tag
                  v-for="name in versionAliasNames(data)"
                  :key="name"
                  :value="name"
                  :severity="name === 'live' ? 'success' : 'secondary'"
                />
                <span v-if="versionAliasNames(data).length === 0" class="no-alias">—</span>
              </div>
            </template>
          </Column>
          <Column header="Actions" style="width: 220px">
            <template #body="{ data }">
              <div class="version-actions">
                <Button
                  v-if="data.status === 'READY' && canPromote"
                  label="Promote to live"
                  size="small"
                  text
                  @click="promoteToLive(data)"
                />
                <Button
                  v-if="data.status === 'READY' && canPromote"
                  label="Point alias…"
                  size="small"
                  text
                  @click="openPointAliasDialog(data)"
                />
                <Button
                  v-if="canManage"
                  label="Retire"
                  size="small"
                  text
                  severity="danger"
                  :disabled="data.status === 'RETIRED'"
                  @click="confirmRetire(data)"
                />
              </div>
            </template>
          </Column>
          <template #empty>No versions published yet.</template>
        </DataTable>
      </FcFormSection>

      <!-- Aliases -->
      <FcFormSection title="Aliases" flat>
        <template v-if="canPromote" #actions>
          <Button label="Point Alias" icon="pi pi-plus" text @click="openPointAliasDialog()" />
        </template>

        <DataTable :value="aliases" size="small">
          <Column field="name" header="Name">
            <template #body="{ data }">
              <Tag :value="data.name" :severity="data.name === 'live' ? 'success' : 'secondary'" />
            </template>
          </Column>
          <Column field="version" header="Version" />
          <Column header="Updated">
            <template #body="{ data }">{{ formatDate(data.updatedAt) }}</template>
          </Column>
          <Column header="Actions" style="width: 80px">
            <template #body="{ data }">
              <Button
                v-if="canPromote"
                v-tooltip="'Remove'"
                icon="pi pi-trash"
                text
                rounded
                severity="danger"
                @click="confirmRemoveAlias(data)"
              />
            </template>
          </Column>
          <template #empty>No aliases yet.</template>
        </DataTable>
      </FcFormSection>

      <!-- What it declares -->
      <FcFormSection
        title="What It Declares"
        :description="
          describeSource
            ? `From version ${describeSource.number}${describeIsLive ? ' (live)' : ' — no version promoted yet'}`
            : 'No published version yet.'
        "
        flat
      >
        <template v-if="describeSource">
          <div class="declare-block">
            <h4>Endpoints</h4>
            <DataTable v-if="declared.endpoints.length" :value="declared.endpoints" size="small">
              <Column field="method" header="Method" style="width: 90px" />
              <Column field="path" header="Path">
                <template #body="{ data }"><code>{{ data.path }}</code></template>
              </Column>
              <Column header="Auth">
                <template #body="{ data }">
                  <Tag :value="data.auth" :severity="authSeverity(data.auth)" />
                </template>
              </Column>
            </DataTable>
            <p v-else class="empty-hint">No endpoints declared.</p>
          </div>

          <div class="declare-block">
            <h4>Subscriptions</h4>
            <ul v-if="declared.subscriptions.length" class="declare-list">
              <li v-for="s in declared.subscriptions" :key="s.eventType + s.path">
                <code>{{ s.eventType }}</code> → <code>{{ s.path }}</code>
                <span v-if="s.mode" class="declare-meta">({{ s.mode }})</span>
              </li>
            </ul>
            <p v-else class="empty-hint">No subscriptions declared.</p>
          </div>

          <div class="declare-block">
            <h4>Schedules</h4>
            <ul v-if="declared.schedules.length" class="declare-list">
              <li v-for="s in declared.schedules" :key="s.cron + s.path">
                <code>{{ s.cron }}</code> → <code>{{ s.path }}</code>
                <span v-if="s.timezone" class="declare-meta">({{ s.timezone }})</span>
              </li>
            </ul>
            <p v-else class="empty-hint">No schedules declared.</p>
          </div>

          <div class="declare-block">
            <h4>Config / Secrets / Databases</h4>
            <div class="chip-row">
              <Tag v-for="k in declared.config" :key="'c' + k" :value="k" severity="secondary" />
              <Tag v-for="k in declared.secrets" :key="'s' + k" :value="k" severity="warn" />
              <Tag v-for="k in declared.db" :key="'d' + k" :value="k" severity="info" />
              <span v-if="!declared.config.length && !declared.secrets.length && !declared.db.length" class="empty-hint">
                None declared.
              </span>
            </div>
          </div>

          <div class="declare-block">
            <h4>Outbound HTTP allow-list</h4>
            <div class="chip-row">
              <Tag v-for="h in declared.httpAllow" :key="h" :value="h" severity="secondary" />
              <span v-if="!declared.httpAllow.length" class="empty-hint">None declared.</span>
            </div>
          </div>

          <div class="declare-block">
            <h4>Emits</h4>
            <div class="chip-row">
              <Tag v-for="e in declared.emits" :key="e" :value="e" severity="secondary" />
              <span v-if="!declared.emits.length" class="empty-hint">None declared.</span>
            </div>
          </div>
        </template>
      </FcFormSection>

      <!-- Settings -->
      <FcFormSection title="Settings" flat>
        <Message v-if="anyMissing" severity="warn" :closable="false" class="missing-banner">
          One or more declared keys have no value set. Promoting a version to
          live is refused while any are missing.
        </Message>

        <div class="declare-block">
          <h4>Config</h4>
          <DataTable :value="configRows" size="small">
            <Column field="key" header="Key">
              <template #body="{ data }"><code>{{ data.key }}</code></template>
            </Column>
            <Column header="Value">
              <template #body="{ data }">
                <span v-if="data.set">{{ data.value }}</span>
                <Tag v-else value="missing" severity="warn" />
              </template>
            </Column>
            <Column header="Actions" style="width: 140px">
              <template #body="{ data }">
                <div v-if="canManage" class="setting-actions">
                  <Button
                    :label="data.set ? 'Edit' : 'Set'"
                    size="small"
                    text
                    @click="openSettingDialog('CONFIG', data.key, data.value)"
                  />
                  <Button
                    v-if="data.set"
                    icon="pi pi-trash"
                    size="small"
                    text
                    severity="danger"
                    @click="confirmRemoveSetting(data)"
                  />
                </div>
              </template>
            </Column>
            <template #empty>No config keys declared.</template>
          </DataTable>
          <Button
            v-if="canManage"
            label="Add Config"
            icon="pi pi-plus"
            text
            size="small"
            @click="openSettingDialog('CONFIG')"
          />
        </div>

        <div class="declare-block">
          <h4>Secrets</h4>
          <p class="empty-hint">Write-only — values are never shown once set.</p>
          <DataTable :value="secretRows" size="small">
            <Column field="key" header="Key">
              <template #body="{ data }"><code>{{ data.key }}</code></template>
            </Column>
            <Column header="Value">
              <template #body="{ data }">
                <Tag v-if="data.set" value="set" severity="success" />
                <Tag v-else value="missing" severity="warn" />
              </template>
            </Column>
            <Column header="Actions" style="width: 140px">
              <template #body="{ data }">
                <div v-if="canManageSecrets" class="setting-actions">
                  <Button
                    :label="data.set ? 'Replace' : 'Set'"
                    size="small"
                    text
                    @click="openSettingDialog('SECRET', data.key)"
                  />
                  <Button
                    v-if="data.set"
                    icon="pi pi-trash"
                    size="small"
                    text
                    severity="danger"
                    @click="confirmRemoveSetting(data)"
                  />
                </div>
              </template>
            </Column>
            <template #empty>No secrets declared.</template>
          </DataTable>
          <Button
            v-if="canManageSecrets"
            label="Add Secret"
            icon="pi pi-plus"
            text
            size="small"
            @click="openSettingDialog('SECRET')"
          />
        </div>

        <div class="declare-block">
          <h4>Databases</h4>
          <p class="empty-hint">Write-only — DSNs are never shown once set.</p>
          <DataTable :value="dbRows" size="small">
            <Column field="key" header="Name">
              <template #body="{ data }"><code>{{ data.key }}</code></template>
            </Column>
            <Column header="Value">
              <template #body="{ data }">
                <Tag v-if="data.set" value="set" severity="success" />
                <Tag v-else value="missing" severity="warn" />
              </template>
            </Column>
            <Column header="Actions" style="width: 140px">
              <template #body="{ data }">
                <div v-if="canManageSecrets" class="setting-actions">
                  <Button
                    :label="data.set ? 'Replace' : 'Set'"
                    size="small"
                    text
                    @click="openSettingDialog('DB', data.key)"
                  />
                  <Button
                    v-if="data.set"
                    icon="pi pi-trash"
                    size="small"
                    text
                    severity="danger"
                    @click="confirmRemoveSetting(data)"
                  />
                </div>
              </template>
            </Column>
            <template #empty>No databases declared.</template>
          </DataTable>
          <Button
            v-if="canManageSecrets"
            label="Add Database"
            icon="pi pi-plus"
            text
            size="small"
            @click="openSettingDialog('DB')"
          />
        </div>
      </FcFormSection>

      <!-- Danger Zone -->
      <FcFormSection v-if="canManage && !editing" title="Danger Zone" flat>
        <div class="action-items">
          <div class="action-item">
            <div class="action-info">
              <strong>Delete Function</strong>
              <p>Removes this function, along with its versions, wiring and artifacts. Cannot be undone.</p>
            </div>
            <Button label="Delete" icon="pi pi-trash" severity="danger" outlined @click="confirmDelete" />
          </div>
        </div>
      </FcFormSection>
    </template>

    <!-- Point Alias Dialog -->
    <Dialog
      v-model:visible="pointAliasDialogVisible"
      header="Point Alias"
      :style="{ width: '420px' }"
      :modal="true"
    >
      <div class="dialog-form">
        <div class="form-field">
          <label>Alias name</label>
          <InputText v-model="pointAliasName" placeholder="live" class="full-width" />
          <small class="hint">e.g. <code>live</code>, <code>qa</code>, <code>canary</code>.</small>
        </div>
        <div class="form-field">
          <label>Version</label>
          <Select
            v-model="pointAliasVersion"
            :options="readyVersionOptions"
            optionLabel="label"
            optionValue="value"
            placeholder="Select a READY version"
            class="full-width"
          />
        </div>
      </div>
      <template #footer>
        <Button
          label="Cancel"
          severity="secondary"
          outlined
          :disabled="pointAliasSubmitting"
          @click="pointAliasDialogVisible = false"
        />
        <Button
          label="Save"
          :loading="pointAliasSubmitting"
          :disabled="!pointAliasName.trim() || pointAliasVersion == null"
          @click="submitPointAlias"
        />
      </template>
    </Dialog>

    <!-- Set Setting Dialog -->
    <Dialog
      v-model:visible="settingDialogVisible"
      :header="`Set ${settingKindLabel(settingDialogKind)}`"
      :style="{ width: '420px' }"
      :modal="true"
    >
      <div class="dialog-form">
        <div class="form-field">
          <label>Key</label>
          <InputText
            v-model="settingDialogKey"
            :disabled="!settingDialogKeyEditable"
            class="full-width"
          />
        </div>
        <div class="form-field">
          <label>Value</label>
          <Textarea v-model="settingDialogValue" rows="2" class="full-width" />
          <small v-if="settingDialogKind !== 'CONFIG'" class="hint">
            Write-only — this value will not be shown again once saved.
          </small>
        </div>
      </div>
      <template #footer>
        <Button
          label="Cancel"
          severity="secondary"
          outlined
          :disabled="settingDialogSubmitting"
          @click="settingDialogVisible = false"
        />
        <Button
          label="Save"
          :loading="settingDialogSubmitting"
          :disabled="!settingDialogKey.trim()"
          @click="submitSettingDialog"
        />
      </template>
    </Dialog>

    <template v-if="editing" #footer>
      <FcFormActions :bordered="false">
        <Button v-if="dirty" label="Discard" severity="secondary" outlined @click="cancelEditing" />
        <Button label="Save" :disabled="!dirty" :loading="saving" @click="saveChanges" />
      </FcFormActions>
    </template>
  </EntityDrawer>
</template>

<style scoped>
.warm-toggle {
  display: flex;
  align-items: center;
  gap: 10px;
}

.action-items {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.action-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 16px;
  background: #fafafa;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.action-info strong {
  display: block;
  margin-bottom: 4px;
}

.action-info p {
  margin: 0;
  font-size: 13px;
  color: #64748b;
}

.digest {
  font-size: 12px;
}

.failure-reason {
  margin-top: 4px;
  font-size: 12px;
  color: #b91c1c;
  max-width: 320px;
}

.alias-chips {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.no-alias {
  color: #94a3b8;
}

.version-actions {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.declare-block {
  margin-bottom: 24px;
}

.declare-block:last-child {
  margin-bottom: 0;
}

.declare-block h4 {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
  color: #475569;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.declare-list {
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
}

.declare-list li {
  margin-bottom: 4px;
}

.declare-meta {
  color: #64748b;
  font-size: 12px;
  margin-left: 4px;
}

.chip-row {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.empty-hint {
  color: #94a3b8;
  font-size: 13px;
  margin: 0 0 8px;
}

.missing-banner {
  margin-bottom: 16px;
}

.setting-actions {
  display: flex;
  gap: 4px;
}

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dialog-form .form-field label {
  display: block;
  font-weight: 500;
  margin-bottom: 6px;
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
</style>
