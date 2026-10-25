<script setup lang="ts">
// The tracing switches of one function: its contexts, their state, and enable / disable.
// Compact enough for the detail drawer; the log itself lives on its own page
// (FunctionTraceLogPage), reached by "View trace logs".
import { toast } from "@/utils/errorBus";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { ApiError } from "@/api/client";
import { functionsApi, type TraceContext } from "@/api/functions";
import { useAuthStore } from "@/stores/auth";
import { userHasPermission } from "@/stores/permissions";
import {
	HTTP_CONTEXT,
	SAMPLE_RATE_MAX,
	SAMPLE_RATE_MIN,
	contextState,
	durationChoices,
	enableBlockedReason,
	indefiniteAllowed,
	indefiniteLabel,
	indefiniteWarning,
	isValidSampleRate,
	sampleLabel,
	traceGates,
} from "@/pages/functions/functionTrace";

const props = withDefaults(
	defineProps<{
		functionId: string;
		/** Show the "View trace logs" button (off on the log page itself). */
		showLogLink?: boolean;
	}>(),
	{ showLogLink: true },
);

const emit = defineEmits<{ (e: "loaded", contexts: TraceContext[]): void }>();

const authStore = useAuthStore();
const gates = computed(() => traceGates((p) => userHasPermission(authStore.user, p)));

// A one-second clock drives the countdowns and the live/off flip.
const now = ref(Date.now());
const clock = setInterval(() => {
	now.value = Date.now();
}, 1000);
onBeforeUnmount(() => clearInterval(clock));

const contexts = ref<TraceContext[]>([]);
const contextsLoading = ref(false);
const contextsError = ref<string | null>(null);
const hasDeclared = computed(() => contexts.value.some((c) => !c.builtIn));

async function loadContexts() {
	if (!gates.value.view) return;
	contextsLoading.value = true;
	contextsError.value = null;
	try {
		contexts.value = (await functionsApi.listTraceContexts(props.functionId)).contexts;
		emit("loaded", contexts.value);
	} catch (e) {
		contextsError.value = e instanceof Error ? e.message : "Failed to load trace contexts";
	} finally {
		contextsLoading.value = false;
	}
}

// The detail drawer is reused between rows.
watch(() => props.functionId, () => void loadContexts(), { immediate: true });

// --- enable / disable ---

const enableVisible = ref(false);
const enableTarget = ref<TraceContext | null>(null);
const enableMinutes = ref(5);
const enableIndefinite = ref(false);
// "Until switched off" must be confirmed before Enable is offered.
const enableIndefiniteConfirmed = ref(false);
const enableSampleRate = ref<number | null>(1);
const enableSubmitting = ref(false);
const enableError = ref<string | null>(null);

// Label/value objects: PrimeVue's Select throws on a function optionLabel.
const enableDurations = computed(() =>
	(enableTarget.value ? durationChoices(enableTarget.value.maxMinutes) : []).map((m) => ({
		label: `${m} minutes`,
		value: m,
	})),
);
const enableSampleValid = computed(() => isValidSampleRate(enableSampleRate.value));
const enableIndefiniteOffered = computed(() => (enableTarget.value ? indefiniteAllowed(enableTarget.value, gates.value) : false));
const enableReady = computed(() => enableSampleValid.value && (!enableIndefinite.value || enableIndefiniteConfirmed.value));
watch(enableIndefinite, () => {
	enableIndefiniteConfirmed.value = false;
});

function openEnable(ctx: TraceContext) {
	enableTarget.value = ctx;
	enableMinutes.value = durationChoices(ctx.maxMinutes)[0] ?? 5;
	enableIndefinite.value = ctx.indefinite === true && indefiniteAllowed(ctx, gates.value);
	enableIndefiniteConfirmed.value = false;
	enableSampleRate.value = 1;
	enableError.value = null;
	enableVisible.value = true;
}

async function submitEnable() {
	const ctx = enableTarget.value;
	if (!ctx || !isValidSampleRate(enableSampleRate.value)) return;
	if (enableIndefinite.value && !enableIndefiniteConfirmed.value) return;
	enableSubmitting.value = true;
	enableError.value = null;
	try {
		await functionsApi.enableTrace(
			props.functionId,
			ctx.name,
			enableIndefinite.value
				? { indefinite: true, sampleRate: enableSampleRate.value }
				: { minutes: enableMinutes.value, sampleRate: enableSampleRate.value },
			{ suppressGlobalErrorToast: true },
		);
		enableVisible.value = false;
		toast.success(
			"Tracing on",
			enableIndefinite.value
				? `${ctx.name} is on until it is switched off (at most ${ctx.indefiniteMaxDays} days)`
				: `${ctx.name} is live for ${enableMinutes.value} minutes`,
		);
		await loadContexts();
	} catch (e) {
		// TRACE_WINDOW_TOO_LONG / TRACE_CONTEXT_UNKNOWN / TRACE_SAMPLE_RATE_INVALID / TRACE_MINUTES_INVALID / TRACE_INDEFINITE_NOT_ALLOWED
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
</script>

<template>
  <div class="trace-controls" data-testid="function-diagnostics">
    <Message v-if="!gates.view" severity="secondary" :closable="false" data-testid="trace-no-permission">
      You do not have permission to view trace logs (platform:function:trace:view).
    </Message>

    <template v-else>
      <p class="intro">
        Tracing is <strong>off by default</strong>. Switch a context on for a set time (or, with the
        permission, a declared context until it is switched off, for a few days at most). The built-in
        <code>http</code> context captures request and response <strong>bodies</strong>, which can
        hold sensitive data, and always turns itself off.
      </p>

      <Message v-if="contextsError" severity="error" :closable="false">{{ contextsError }}</Message>

      <ul class="contexts" data-testid="trace-contexts">
        <li v-for="c in contexts" :key="c.name" class="context" :data-testid="`trace-context-${c.name}`">
          <div class="context-main">
            <div class="context-name">
              <code>{{ c.name }}</code>
              <Tag v-if="c.builtIn" value="built-in" severity="secondary" />
            </div>
            <p class="context-desc">{{ c.description || "No description." }}</p>
            <p class="context-state">
              <template v-if="contextState(c, now).live">
                <Tag value="Live" severity="success" />
                <span v-if="contextState(c, now).indefinite" class="countdown" :title="`Switches off by itself at ${c.enabledUntil}`">
                  until switched off <span class="sample">(ends by itself {{ contextState(c, now).label }})</span>
                </span>
                <span v-else class="countdown" :title="`Until ${c.enabledUntil}`">{{ contextState(c, now).label }} left</span>
                <span class="sample">{{ sampleLabel(c.sampleRate) }}</span>
              </template>
              <Tag v-else value="Off" severity="secondary" />
            </p>
            <small
              v-if="c.name === HTTP_CONTEXT && enableBlockedReason(c.name, gates)"
              class="hint"
              data-testid="trace-http-blocked"
            >
              {{ enableBlockedReason(c.name, gates) }}
            </small>
          </div>
          <div v-if="gates.manage" class="context-actions">
            <Button
              :label="contextState(c, now).live ? 'Change' : 'Enable'"
              size="small"
              outlined
              :disabled="enableBlockedReason(c.name, gates) !== null"
              :data-testid="`trace-enable-${c.name}`"
              @click="openEnable(c)"
            />
            <Button
              v-if="contextState(c, now).live"
              label="Disable"
              size="small"
              text
              severity="danger"
              :data-testid="`trace-disable-${c.name}`"
              @click="disable(c)"
            />
          </div>
        </li>
        <li v-if="!contextsLoading && contexts.length === 0" class="text-muted">No contexts.</li>
      </ul>

      <Message v-if="!contextsLoading && !contextsError && !hasDeclared" severity="info" :closable="false" data-testid="trace-no-declared">
        This function declares no trace contexts yet, so only <code>http</code> is available (it is
        always available). To add your own, list their names under <code>traceContexts</code> in the
        function's describe document and publish a new version; the SDK's <code>trace</code> and
        <code>tracing</code> helpers write to them.
      </Message>

      <div v-if="showLogLink" class="log-link">
        <router-link :to="{ name: 'function-traces', params: { id: functionId } }" custom v-slot="{ navigate, href }">
          <Button
            as="a"
            :href="href"
            label="View trace logs"
            icon="pi pi-external-link"
            data-testid="trace-view-logs"
            @click="navigate"
          />
        </router-link>
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
          This records request and response bodies, which can hold sensitive data. It turns itself off
          when the time is up.
        </Message>
        <div class="form-field">
          <label for="trace-minutes">Stay on for</label>
          <Select
            v-model="enableMinutes"
            input-id="trace-minutes"
            :options="enableDurations"
            option-label="label"
            option-value="value"
            :disabled="enableIndefinite"
            class="full-width"
          />
          <small v-if="!enableIndefinite" class="hint">At most {{ enableTarget?.maxMinutes }} minutes for this context.</small>
        </div>
        <div v-if="enableIndefiniteOffered" class="form-field check">
          <Checkbox v-model="enableIndefinite" input-id="trace-indefinite" binary data-testid="trace-indefinite" />
          <label for="trace-indefinite">{{ indefiniteLabel(enableTarget?.indefiniteMaxDays ?? 0) }}</label>
        </div>
        <div v-if="enableIndefiniteOffered && enableIndefinite" class="form-field" data-testid="trace-indefinite-confirm">
          <Message severity="warn" :closable="false">{{ indefiniteWarning(enableTarget?.indefiniteMaxDays ?? 0) }}</Message>
          <div class="check confirm">
            <Checkbox v-model="enableIndefiniteConfirmed" input-id="trace-indefinite-confirm" binary data-testid="trace-indefinite-confirmed" />
            <label for="trace-indefinite-confirm">I understand, keep it on</label>
          </div>
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
        <Button label="Enable" :loading="enableSubmitting" :disabled="!enableReady" data-testid="trace-enable-submit" @click="submitEnable" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.trace-controls {
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

.contexts {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.context {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 8px 16px;
  padding: 12px 14px;
}

.context + .context {
  border-top: 1px solid #e5e7eb;
}

.context-main {
  flex: 1 1 260px;
  min-width: 0;
}

.context-name {
  display: flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

.context-desc {
  margin: 4px 0 6px;
  font-size: 13px;
  color: #64748b;
}

.context-state {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-size: 13px;
}

.countdown {
  font-variant-numeric: tabular-nums;
}

.sample {
  color: #64748b;
}

.context-actions {
  display: flex;
  gap: 4px;
  flex: 0 0 auto;
}

.log-link {
  display: flex;
}

.log-link a {
  text-decoration: none;
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

.form-field.check {
  display: flex;
  align-items: center;
  gap: 8px;
}

.form-field.check label,
.check.confirm label {
  margin: 0;
  font-weight: 400;
}

.check.confirm {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
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
