<script setup lang="ts">
// The trace log of one function, on a page of its own: the records are wide
// (http request and response panels), so a drawer is too narrow for them.
import { toast } from "@/utils/errorBus";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useRoute } from "vue-router";
import FunctionTraceControls from "@/pages/functions/FunctionTraceControls.vue";
import { functionsApi, type FunctionRecord, type TraceContext, type TraceLogRecord } from "@/api/functions";
import { useAuthStore } from "@/stores/auth";
import { userHasPermission } from "@/stores/permissions";
import {
	BODIES_HIDDEN_MESSAGE,
	KIND_CHOICES,
	LEVEL_CHOICES,
	SINCE_CHOICES,
	anyLive,
	bodyView,
	buildTraceQuery,
	dataText,
	emptyFilters,
	hasActiveFilters,
	headerRows,
	httpSummary,
	invocationCounts,
	levelSeverity,
	localTime,
	parseHttpData,
	shortId,
	traceGates,
	type BodyView,
} from "@/pages/functions/functionTrace";

const route = useRoute();
const functionId = computed(() => String(route.params["id"]));

const REFRESH_MS = 5000;

const authStore = useAuthStore();
const gates = computed(() => traceGates((p) => userHasPermission(authStore.user, p)));

// A one-second clock drives the live/off flip.
const now = ref(Date.now());
const clock = setInterval(() => {
	now.value = Date.now();
}, 1000);

const fnRecord = ref<FunctionRecord | null>(null);
const contexts = ref<TraceContext[]>([]);

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

const live = computed(() => anyLive(contexts.value, now.value));
const counts = computed(() => invocationCounts(records.value));
const contextOptions = computed(() => contexts.value.map((c) => c.name));

function onContextsLoaded(list: TraceContext[]) {
	contexts.value = list;
}

// The switches panel may be collapsed (not mounted), so the page keeps its own copy of the
// contexts: they drive the context filter and the live flag that starts auto refresh.
async function loadContexts() {
	if (!gates.value.view) return;
	try {
		contexts.value = (await functionsApi.listTraceContexts(functionId.value)).contexts;
	} catch {
		// the switches panel reports its own load errors
	}
}

/** `quiet` refreshes (auto refresh) keep the table in place instead of showing a spinner. */
async function loadLog(quiet = false) {
	if (!gates.value.view) return;
	const seq = ++logSeq;
	if (!quiet) logLoading.value = true;
	try {
		const page = await functionsApi.listTraceLog(
			functionId.value,
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
			functionId.value,
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

// Navigating between functions reloads everything.
watch(
	functionId,
	(id) => {
		filters.value = emptyFilters();
		expandedRows.value = {};
		records.value = [];
		nextCursor.value = null;
		fnRecord.value = null;
		void functionsApi.get(id).then((f) => (fnRecord.value = f)).catch(() => undefined);
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

const ctxTimer = setInterval(() => void loadContexts(), 15000);

onBeforeUnmount(() => {
	clearInterval(ctxTimer);
	clearInterval(clock);
	clearInterval(refreshTimer);
	clearTimeout(filterTimer);
});

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
  <div class="page-container">
    <header class="page-header">
      <div>
        <router-link :to="{ name: 'function-detail', params: { id: functionId } }" class="back" data-testid="trace-back">
          <i class="pi pi-arrow-left" /> Back to function
        </router-link>
        <h1 class="page-title">Trace logs</h1>
        <p class="page-subtitle"><code v-if="fnRecord">{{ fnRecord.address }}</code><span v-else>&nbsp;</span></p>
      </div>
    </header>

    <div class="diagnostics" data-testid="function-trace-page">
      <Message v-if="!gates.view" severity="secondary" :closable="false" data-testid="trace-no-permission">
        You do not have permission to view trace logs (platform:function:trace:view).
      </Message>

      <template v-else>
        <Panel header="Tracing switches" toggleable collapsed class="fc-card switches">
          <FunctionTraceControls :function-id="functionId" :show-log-link="false" @loaded="onContextsLoaded" />
        </Panel>

        <div class="fc-card log-card">
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

        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.back {
  display: inline-block;
  margin-bottom: 6px;
  font-size: 13px;
  text-decoration: none;
  color: #475569;
}

.back:hover {
  color: #0f172a;
}

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
