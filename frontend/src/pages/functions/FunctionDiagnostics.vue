<script setup lang="ts">
import { toast } from "@/utils/errorBus";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { ApiError } from "@/api/client";
import {
	functionsApi,
	type TraceContext,
	type TraceLogRecord,
} from "@/api/functions";
import { useAuthStore } from "@/stores/auth";
import { userHasPermission } from "@/stores/permissions";
import {
	BODIES_HIDDEN_MESSAGE,
	HTTP_CONTEXT,
	KIND_CHOICES,
	LEVEL_CHOICES,
	SAMPLE_RATE_MAX,
	SAMPLE_RATE_MIN,
	SINCE_CHOICES,
	anyLive,
	bodyView,
	buildTraceQuery,
	countdown,
	dataText,
	durationChoices,
	emptyFilters,
	enableBlockedReason,
	hasActiveFilters,
	headerRows,
	httpSummary,
	invocationCounts,
	isValidSampleRate,
	levelSeverity,
	localTime,
	parseHttpData,
	sampleLabel,
	shortId,
	traceGates,
	type BodyView,
} from "@/pages/functions/functionTrace";

const props = defineProps<{
	functionId: string;
}>();

const REFRESH_MS = 5000;

const authStore = useAuthStore();
const gates = computed(() => traceGates((p) => userHasPermission(authStore.user, p)));

// A one-second clock drives the countdowns and the live/off flip.
const now = ref(Date.now());
const clock = setInterval(() => {
	now.value = Date.now();
}, 1000);

const contexts = ref<TraceContext[]>([]);
const contextsLoading = ref(false);
const contextsError = ref<string | null>(null);

const records = ref<TraceLogRecord[]>([]);
const nextCursor = ref<string | null>(null);
const logLoading = ref(false);
const loadingMore = ref(false);
const logError = ref<string | null>(null);
const filters = ref(emptyFilters());
const expandedRows = ref<Record<string, boolean>>({});
const paused = ref(false);

// Responses can overtake each other (filters change mid-flight); only the newest applies.
let logSeq = 0;

const hasDeclared = computed(() => contexts.value.some((c) => !c.builtIn));
const live = computed(() => anyLive(contexts.value, now.value));
const counts = computed(() => invocationCounts(records.value));
const contextOptions = computed(() => contexts.value.map((c) => c.name));

async function loadContexts() {
	if (!gates.value.view) return;
	contextsLoading.value = true;
	contextsError.value = null;
	try {
		contexts.value = (await functionsApi.listTraceContexts(props.functionId)).contexts;
	} catch (e) {
		contextsError.value = e instanceof Error ? e.message : "Failed to load trace contexts";
	} finally {
		contextsLoading.value = false;
	}
}

/** `quiet` refreshes (auto refresh) keep the table in place instead of showing a spinner. */
async function loadLog(quiet = false) {
	if (!gates.value.view) return;
	const seq = ++logSeq;
	if (!quiet) logLoading.value = true;
	try {
		const page = await functionsApi.listTraceLog(
			props.functionId,
			buildTraceQuery(filters.value, Date.now()),
			{ suppressGlobalErrorToast: true },
		);
		if (seq !== logSeq) return;
		records.value = page.records;
		nextCursor.value = page.nextCursor;
		logError.value = null;
	} catch (e) {
		if (seq !== logSeq) return;
		logError.value = e instanceof Error ? e.message : "Failed to load trace log";
	} finally {
		if (seq === logSeq) logLoading.value = false;
	}
}

async function loadMore() {
	if (!nextCursor.value) return;
	loadingMore.value = true;
	try {
		const page = await functionsApi.listTraceLog(
			props.functionId,
			buildTraceQuery(filters.value, Date.now(), { cursor: nextCursor.value }),
		);
		const seen = new Set(records.value.map((r) => r.id));
		records.value = [...records.value, ...page.records.filter((r) => !seen.has(r.id))];
		nextCursor.value = page.nextCursor;
	} catch {
		// the global toast reports it; the cursor stays so the button can retry
	} finally {
		loadingMore.value = false;
	}
}

// Switching functions reloads everything; the drawer instance is reused between rows.
watch(
	() => props.functionId,
	() => {
		filters.value = emptyFilters();
		expandedRows.value = {};
		records.value = [];
		nextCursor.value = null;
		void loadContexts();
		void loadLog();
	},
	{ immediate: true },
);

let filterTimer: ReturnType<typeof setTimeout> | undefined;
watch(
	filters,
	() => {
		clearTimeout(filterTimer);
		filterTimer = setTimeout(() => void loadLog(), 300);
	},
	{ deep: true },
);

// Auto refresh while any context is live and not paused. Re-checks `live` each
// tick, so it stops by itself when the window passes.
let refreshTimer: ReturnType<typeof setInterval> | undefined;
watch(
	[live, paused],
	([isLive, isPaused]) => {
		clearInterval(refreshTimer);
		refreshTimer = undefined;
		if (isLive && !isPaused) {
			refreshTimer = setInterval(() => void loadLog(true), REFRESH_MS);
		}
	},
	{ immediate: true },
);

onBeforeUnmount(() => {
	clearInterval(clock);
	clearInterval(refreshTimer);
	clearTimeout(filterTimer);
});

// --- enable / disable ---

const enableVisible = ref(false);
const enableTarget = ref<TraceContext | null>(null);
const enableMinutes = ref(5);
const enableSampleRate = ref<number | null>(1);
const enableSubmitting = ref(false);
const enableError = ref<string | null>(null);

const enableDurations = computed(() =>
	enableTarget.value ? durationChoices(enableTarget.value.maxMinutes) : [],
);
const enableSampleValid = computed(() => isValidSampleRate(enableSampleRate.value));

function openEnable(ctx: TraceContext) {
	enableTarget.value = ctx;
	enableMinutes.value = durationChoices(ctx.maxMinutes)[0] ?? 5;
	enableSampleRate.value = 1;
	enableError.value = null;
	enableVisible.value = true;
}

async function submitEnable() {
	const ctx = enableTarget.value;
	if (!ctx || !isValidSampleRate(enableSampleRate.value)) return;
	enableSubmitting.value = true;
	enableError.value = null;
	try {
		await functionsApi.enableTrace(
			props.functionId,
			ctx.name,
			{ minutes: enableMinutes.value, sampleRate: enableSampleRate.value },
			{ suppressGlobalErrorToast: true },
		);
		enableVisible.value = false;
		toast.success("Tracing on", `${ctx.name} is live for ${enableMinutes.value} minutes`);
		await loadContexts();
		void loadLog(true);
	} catch (e) {
		// TRACE_WINDOW_TOO_LONG / TRACE_CONTEXT_UNKNOWN / TRACE_SAMPLE_RATE_INVALID / TRACE_MINUTES_INVALID
		enableError.value = e instanceof ApiError || e instanceof Error ? e.message : "Failed to enable tracing";
	} finally {
		enableSubmitting.value = false;
	}
}

async function disable(ctx: TraceContext) {
	try {
		await functionsApi.disableTrace(props.functionId, ctx.name);
		toast.success("Tracing off", `${ctx.name} is off`);
		await loadContexts();
	} catch {
		// the global toast reports it
	}
}

// --- log viewer helpers ---

function filterBy(field: "invocationId" | "reference" | "correlationId", value: string | null) {
	if (value) filters.value = { ...filters.value, [field]: value };
}

function clearFilters() {
	filters.value = emptyFilters();
}

async function copy(text: string) {
	try {
		await navigator.clipboard.writeText(text);
		toast.success("Copied", "Copied to clipboard");
	} catch {
		toast.error("Copy failed", "Clipboard is not available");
	}
}

function httpOf(r: TraceLogRecord) {
	return r.kind === "http" ? parseHttpData(r.data) : null;
}

interface Panel {
	title: string;
	line: string;
	headers: [string, string][];
	body: BodyView;
	raw: string;
}

function panelsOf(r: TraceLogRecord): Panel[] {
	const d = httpOf(r);
	if (!d) return [];
	const redacted = r.bodiesRedacted === true;
	const req = bodyView(d.request, redacted);
	const res = bodyView(d.response, redacted);
	return [
		{
			title: "Request",
			line: `${d.request.method ?? ""} ${d.request.path ?? ""}${d.request.rawQuery ? `?${d.request.rawQuery}` : ""}`.trim(),
			headers: headerRows(d.request.headers),
			body: req,
			raw: d.request.body ?? "",
		},
		{
			title: "Response",
			line: d.response.status != null ? `Status ${d.response.status}` : "",
			headers: headerRows(d.response.headers),
			body: res,
			raw: d.response.body ?? "",
		},
	];
}
</script>

<template>
  <div class="diagnostics" data-testid="function-diagnostics">
    <Message v-if="!gates.view" severity="secondary" :closable="false" data-testid="trace-no-permission">
      You do not have permission to view trace logs (platform:function:trace:view).
    </Message>

    <template v-else>
      <p class="intro">
        Tracing is <strong>off by default</strong>. A context you switch on stays live for the chosen
        time and then turns itself off. The built-in <code>http</code> context captures request and
        response <strong>bodies</strong>, which can hold sensitive data.
      </p>

      <Message v-if="contextsError" severity="error" :closable="false">{{ contextsError }}</Message>

      <!-- Contexts -->
      <DataTable :value="contexts" size="small" :loading="contextsLoading" data-key="name" data-testid="trace-contexts">
        <Column header="Context">
          <template #body="{ data }">
            <code>{{ data.name }}</code>
            <Tag v-if="data.builtIn" value="built-in" severity="secondary" class="ctx-tag" />
          </template>
        </Column>
        <Column header="Description">
          <template #body="{ data }">
            <span class="text-muted">{{ data.description || "—" }}</span>
          </template>
        </Column>
        <Column header="State" style="width: 190px">
          <template #body="{ data }">
            <template v-if="countdown(data.enabledUntil, now).live">
              <Tag value="Live" severity="success" />
              <span class="countdown" :title="`Until ${data.enabledUntil}`">{{ countdown(data.enabledUntil, now).label }}</span>
            </template>
            <Tag v-else value="Off" severity="secondary" />
          </template>
        </Column>
        <Column header="Sampling" style="width: 130px">
          <template #body="{ data }">
            <span v-if="countdown(data.enabledUntil, now).live">{{ sampleLabel(data.sampleRate) }}</span>
            <span v-else class="text-muted">—</span>
          </template>
        </Column>
        <Column v-if="gates.manage" header="Actions" style="width: 200px">
          <template #body="{ data }">
            <div class="ctx-actions">
              <Button
                :label="countdown(data.enabledUntil, now).live ? 'Change' : 'Enable'"
                size="small"
                text
                :disabled="enableBlockedReason(data.name, gates) !== null"
                :data-testid="`trace-enable-${data.name}`"
                @click="openEnable(data)"
              />
              <Button
                v-if="countdown(data.enabledUntil, now).live"
                label="Disable"
                size="small"
                text
                severity="danger"
                :data-testid="`trace-disable-${data.name}`"
                @click="disable(data)"
              />
            </div>
            <small
              v-if="data.name === HTTP_CONTEXT && enableBlockedReason(data.name, gates)"
              class="hint"
              data-testid="trace-http-blocked"
            >
              {{ enableBlockedReason(data.name, gates) }}
            </small>
          </template>
        </Column>
        <template #empty>No contexts.</template>
      </DataTable>

      <Message v-if="!contextsLoading && !contextsError && !hasDeclared" severity="info" :closable="false" class="declare-hint" data-testid="trace-no-declared">
        This function declares no trace contexts yet, so only <code>http</code> is available (it is
        always available). To add your own, list their names under <code>traceContexts</code> in the
        function's describe document and publish a new version; the SDK's <code>trace</code> and
        <code>tracing</code> helpers write to them.
      </Message>

      <!-- Log viewer -->
      <div class="log-header">
        <h4>Trace log</h4>
        <div class="log-header-actions">
          <Tag v-if="live && !paused" value="Auto refresh every 5 s" severity="success" />
          <Tag v-else-if="live && paused" value="Auto refresh paused" severity="warn" />
          <Button
            v-if="live"
            :label="paused ? 'Resume' : 'Pause'"
            :icon="paused ? 'pi pi-play' : 'pi pi-pause'"
            size="small"
            text
            data-testid="trace-pause"
            @click="paused = !paused"
          />
          <Button icon="pi pi-refresh" label="Refresh" size="small" text :loading="logLoading" @click="loadLog()" />
        </div>
      </div>

      <div class="filters" data-testid="trace-filters">
        <Select v-model="filters.context" :options="contextOptions" showClear placeholder="Context" />
        <Select v-model="filters.kind" :options="[...KIND_CHOICES]" showClear placeholder="Kind" />
        <Select v-model="filters.level" :options="[...LEVEL_CHOICES]" showClear placeholder="Level" />
        <Select
          v-model="filters.sinceMinutes"
          :options="[...SINCE_CHOICES]"
          optionLabel="label"
          optionValue="minutes"
        />
        <InputText v-model="filters.reference" placeholder="Reference" />
        <InputText v-model="filters.correlationId" placeholder="Correlation id" />
        <InputText v-model="filters.invocationId" placeholder="Invocation id" />
        <Button v-if="hasActiveFilters(filters)" label="Clear" icon="pi pi-filter-slash" text size="small" @click="clearFilters" />
      </div>

      <Message v-if="logError" severity="error" :closable="false" data-testid="trace-log-error">{{ logError }}</Message>

      <DataTable
        v-model:expandedRows="expandedRows"
        :value="records"
        size="small"
        data-key="id"
        :loading="logLoading"
        data-testid="trace-log"
      >
        <Column expander style="width: 40px" />
        <Column header="Time" style="width: 170px">
          <template #body="{ data }">
            <span :title="data.at">{{ localTime(data.at) }}</span>
          </template>
        </Column>
        <Column header="Context" style="width: 100px">
          <template #body="{ data }"><code>{{ data.context }}</code></template>
        </Column>
        <Column header="Kind" style="width: 120px">
          <template #body="{ data }">
            <Tag :value="data.kind" severity="secondary" />
            <Tag :value="data.level" :severity="levelSeverity(data.level)" class="ctx-tag" />
          </template>
        </Column>
        <Column header="Message">
          <template #body="{ data }">
            <span v-if="httpOf(data)">{{ httpSummary(httpOf(data)!) }}</span>
            <span v-else>{{ data.message }}</span>
          </template>
        </Column>
        <Column header="Reference" style="width: 130px">
          <template #body="{ data }">
            <a v-if="data.reference" class="link" title="Filter to this reference" @click.stop="filterBy('reference', data.reference)">{{ data.reference }}</a>
          </template>
        </Column>
        <Column header="Correlation" style="width: 130px">
          <template #body="{ data }">
            <a v-if="data.correlationId" class="link" :title="data.correlationId" @click.stop="filterBy('correlationId', data.correlationId)">{{ shortId(data.correlationId) }}</a>
          </template>
        </Column>
        <Column header="Invocation" style="width: 150px">
          <template #body="{ data }">
            <a class="link" :title="`${data.invocationId} (click to show only this invocation)`" @click.stop="filterBy('invocationId', data.invocationId)">{{ shortId(data.invocationId) }}</a>
            <Tag :value="`${counts.get(data.invocationId) ?? 1}`" severity="secondary" class="ctx-tag" :title="`${counts.get(data.invocationId) ?? 1} records in this view`" />
          </template>
        </Column>
        <template #expansion="{ data }">
          <div class="expansion">
            <div class="meta">
              <span>v{{ data.version }}</span>
              <span v-if="data.runnerId">runner {{ data.runnerId }}</span>
              <span :title="localTime(data.at)">{{ data.at }}</span>
            </div>

            <template v-if="httpOf(data)">
              <p class="http-line" data-testid="trace-http-line">{{ httpSummary(httpOf(data)!) }}</p>
              <Message v-if="data.bodiesRedacted" severity="warn" :closable="false" data-testid="trace-bodies-hidden">
                {{ BODIES_HIDDEN_MESSAGE }}
              </Message>
              <div class="panels">
                <section v-for="p in panelsOf(data)" :key="p.title" class="panel">
                  <h5>{{ p.title }}<span v-if="p.line" class="panel-line">{{ p.line }}</span></h5>
                  <table v-if="p.headers.length" class="headers">
                    <tbody>
                      <tr v-for="[k, v] in p.headers" :key="k">
                        <th>{{ k }}</th>
                        <td>{{ v }}</td>
                      </tr>
                    </tbody>
                  </table>
                  <div v-if="p.body.state === 'shown'" class="body-block">
                    <div class="body-head">
                      <Tag :value="p.body.label" severity="secondary" />
                      <small v-if="p.body.note">{{ p.body.note }}</small>
                      <Button icon="pi pi-copy" size="small" text title="Copy body" @click="copy(p.raw)" />
                    </div>
                    <pre class="mono">{{ p.body.text }}</pre>
                  </div>
                  <small v-else-if="p.body.state === 'empty'" class="text-muted">
                    No body{{ p.body.note ? ` (${p.body.note})` : "" }}
                  </small>
                </section>
              </div>
            </template>

            <template v-else>
              <p class="message">{{ data.message }}</p>
              <div v-if="dataText(data.data) !== null" class="body-block">
                <div class="body-head">
                  <Tag value="data" severity="secondary" />
                  <Button icon="pi pi-copy" size="small" text title="Copy data" @click="copy(dataText(data.data)!)" />
                </div>
                <pre class="mono">{{ dataText(data.data) }}</pre>
              </div>
            </template>
          </div>
        </template>
        <template #empty>
          <span v-if="hasActiveFilters(filters)">No records match these filters.</span>
          <span v-else>No trace records yet. Switch a context on above and invoke the function.</span>
        </template>
      </DataTable>

      <div v-if="nextCursor" class="load-more">
        <Button label="Load more" severity="secondary" outlined size="small" :loading="loadingMore" data-testid="trace-load-more" @click="loadMore" />
      </div>
    </template>

    <!-- Enable dialog -->
    <Dialog
      v-model:visible="enableVisible"
      :header="`Enable tracing: ${enableTarget?.name ?? ''}`"
      :style="{ width: '26rem', maxWidth: 'calc(100vw - 2rem)' }"
      :modal="true"
    >
      <div class="dialog-form">
        <Message v-if="enableTarget?.name === HTTP_CONTEXT" severity="warn" :closable="false">
          This records request and response bodies, which can hold sensitive data. It turns itself
          off when the time is up.
        </Message>
        <div class="form-field">
          <label for="trace-minutes">Stay on for</label>
          <Select
            v-model="enableMinutes"
            input-id="trace-minutes"
            :options="enableDurations"
            :option-label="(m: number) => `${m} minutes`"
            class="full-width"
          />
          <small class="hint">At most {{ enableTarget?.maxMinutes }} minutes for this context.</small>
        </div>
        <div class="form-field">
          <label for="trace-sample">Sample rate (1 in N)</label>
          <InputNumber
            v-model="enableSampleRate"
            input-id="trace-sample"
            :min="SAMPLE_RATE_MIN"
            :max="SAMPLE_RATE_MAX"
            show-buttons
            class="full-width"
          />
          <small class="hint">1 captures every invocation; 10 captures one in ten.</small>
        </div>
        <Message v-if="enableError" severity="error" :closable="false" data-testid="trace-enable-error">{{ enableError }}</Message>
      </div>
      <template #footer>
        <Button label="Cancel" severity="secondary" outlined :disabled="enableSubmitting" @click="enableVisible = false" />
        <Button label="Enable" :loading="enableSubmitting" :disabled="!enableSampleValid" @click="submitEnable" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.diagnostics {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.intro {
  margin: 0;
  font-size: 13px;
  color: #475569;
}

.text-muted {
  color: #64748b;
}

.ctx-tag {
  margin-left: 6px;
}

.countdown {
  margin-left: 8px;
  font-variant-numeric: tabular-nums;
  font-size: 13px;
}

.ctx-actions {
  display: flex;
  gap: 4px;
}

.declare-hint {
  margin: 0;
}

.log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
}

.log-header h4 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: #475569;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.log-header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.filters > * {
  min-width: 140px;
}

.link {
  cursor: pointer;
  color: #2563eb;
  font-size: 12px;
  word-break: break-all;
}

.link:hover {
  text-decoration: underline;
}

.expansion {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 4px 8px 12px;
}

.meta {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: #64748b;
}

.http-line,
.message {
  margin: 0;
  font-weight: 500;
}

.panels {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 12px;
}

.panel {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 10px 12px;
  background: #fafafa;
}

.panel h5 {
  margin: 0 0 8px;
  font-size: 13px;
}

.panel-line {
  margin-left: 8px;
  font-weight: 400;
  font-family: monospace;
  color: #475569;
}

.headers {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  margin-bottom: 8px;
}

.headers th {
  text-align: left;
  font-weight: 500;
  padding: 2px 8px 2px 0;
  vertical-align: top;
  white-space: nowrap;
  color: #475569;
}

.headers td {
  padding: 2px 0;
  word-break: break-all;
}

.body-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.body-head :deep(.p-button) {
  margin-left: auto;
}

.mono {
  margin: 4px 0 0;
  padding: 8px;
  max-height: 320px;
  overflow: auto;
  font-family: monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
}

.load-more {
  display: flex;
  justify-content: center;
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
