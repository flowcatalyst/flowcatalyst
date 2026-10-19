<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ApiError } from "@/api/client";
import {
	dispatchJobsApi,
	type DispatchJobFilterBody,
	type RequeueByFilterPreviewResponse,
	type RequeueByFilterResponse,
} from "@/api/dispatch-jobs";
import { toast } from "@/utils/errorBus";
import {
	REQUEUE_BY_FILTER_MAX,
	plural,
	statusSeverity,
	type FilterSummaryItem,
} from "./requeueByFilter";

// Confirm step of "requeue all matching": previews the filter (count, by
// status, how many this caller may requeue), then runs it with that eligible
// count as confirmCount. A 409 FILTER_MATCH_CHANGED (more jobs match now)
// re-previews and asks again; nothing was reset.

const visible = defineModel<boolean>("visible", { required: true });
const props = defineProps<{
	filter: DispatchJobFilterBody;
	summary: FilterSummaryItem[];
}>();
const emit = defineEmits<{
	previewed: [preview: RequeueByFilterPreviewResponse];
	/** The run finished (result) or failed part-way (null) — reload the list. */
	done: [result: RequeueByFilterResponse | null];
}>();

const preview = ref<RequeueByFilterPreviewResponse | null>(null);
const loading = ref(false);
const running = ref(false);
const changed = ref(false);
let seq = 0;

async function loadPreview() {
	const mine = ++seq;
	loading.value = true;
	preview.value = null;
	try {
		const p = await dispatchJobsApi.previewRequeueByFilter(props.filter);
		if (mine !== seq) return;
		preview.value = p;
		emit("previewed", p);
	} catch {
		// apiFetch has already shown the error.
	} finally {
		if (mine === seq) loading.value = false;
	}
}

watch(visible, (open) => {
	if (open) {
		changed.value = false;
		void loadPreview();
	}
});

const statusEntries = computed(() =>
	Object.entries(preview.value?.byStatus ?? {}).sort((a, b) => b[1] - a[1]),
);
// Matches a requeue leaves alone for reasons other than the two below: other
// tenants' and platform-scoped jobs.
const outOfScope = computed(() =>
	preview.value
		? preview.value.count - preview.value.eligibleCount - preview.value.completedCount - preview.value.inFlightCount
		: 0,
);
// The time window the counts cover; with no dates chosen the server holds the
// request to the last 24 hours.
const windowText = computed(() => {
	const w = preview.value?.window;
	if (!w) return "";
	const fmt = (d: string) => new Date(d).toLocaleString();
	return `${fmt(w.from)} – ${fmt(w.to)}${w.defaulted ? " (the last 24 hours: no dates chosen)" : ""}`;
});
const toRequeue = computed(() =>
	preview.value ? Math.min(preview.value.eligibleCount, REQUEUE_BY_FILTER_MAX) : 0,
);
const canConfirm = computed(
	() => !!preview.value && !loading.value && !running.value && preview.value.eligibleCount > 0,
);

async function confirm() {
	if (!preview.value || !canConfirm.value) return;
	running.value = true;
	changed.value = false;
	try {
		const res = await dispatchJobsApi.requeueByFilter(props.filter, preview.value.eligibleCount);
		visible.value = false;
		emit("done", res);
	} catch (error) {
		if (error instanceof ApiError && error.status === 409 && error.code === "FILTER_MATCH_CHANGED") {
			changed.value = true;
			await loadPreview();
		} else {
			toast.error("Requeue failed", error instanceof Error ? error.message : undefined);
			// Batches before a failure stay committed: the list is stale either way.
			emit("done", null);
		}
	} finally {
		running.value = false;
	}
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    header="Requeue all matching jobs"
    modal
    :closable="!running"
    :style="{ width: '560px' }"
    :breakpoints="{ '640px': '95vw' }"
  >
    <div class="rbf">
      <section>
        <h4 class="rbf-heading">Filters</h4>
        <dl v-if="summary.length" class="rbf-filters">
          <template v-for="item in summary" :key="item.label">
            <dt>{{ item.label }}</dt>
            <dd>{{ item.value }}</dd>
          </template>
        </dl>
        <p v-else class="rbf-nofilter">
          <i class="pi pi-exclamation-triangle" />
          No filters — this matches every dispatch job you can see.
        </p>
      </section>

      <section>
        <h4 class="rbf-heading">Matching jobs</h4>
        <div v-if="loading" class="rbf-loading">
          <ProgressSpinner style="width: 1.25rem; height: 1.25rem" strokeWidth="6" />
          Counting matching jobs…
        </div>
        <template v-else-if="preview">
          <p v-if="windowText" class="rbf-line text-muted">Created {{ windowText }}.</p>
          <p class="rbf-line">
            <strong>{{ plural(preview.count, "job") }}</strong> match.
          </p>
          <div v-if="statusEntries.length" class="rbf-statuses">
            <Tag
              v-for="[status, n] in statusEntries"
              :key="status"
              :value="`${status} · ${n.toLocaleString()}`"
              :severity="statusSeverity(status)"
            />
          </div>
          <p class="rbf-line">
            <strong>{{ plural(preview.eligibleCount, "job") }}</strong> can be requeued.
            <span v-if="outOfScope > 0" class="text-muted">
              {{ outOfScope.toLocaleString() }} platform-scoped or outside your client access will be left alone.
            </span>
          </p>
          <p v-if="preview.inFlightCount > 0" class="rbf-line text-muted">
            {{ plural(preview.inFlightCount, "job") }} QUEUED or PROCESSING will be left alone — a job in flight is never requeued.
          </p>
          <p v-if="preview.completedCount > 0" class="rbf-line text-muted">
            {{ plural(preview.completedCount, "COMPLETED job") }} will be left alone — completed jobs are redriven only through the API
            (redriveCompleted and confirmRedriveCompleted, with the redrive-completed permission).
          </p>
          <p v-if="preview.eligibleCount > REQUEUE_BY_FILTER_MAX" class="rbf-line text-muted">
            At most {{ REQUEUE_BY_FILTER_MAX.toLocaleString() }} are requeued per run; run it again for the rest.
          </p>
        </template>
        <p v-else class="rbf-line text-muted">Could not count the matching jobs.</p>
      </section>

      <Message v-if="changed" severity="warn" :closable="false">
        More jobs match than when you looked, so nothing was requeued. The counts above are current — review them and confirm again.
      </Message>
      <Message severity="warn" :closable="false">
        Requeue resets each job to <strong>PENDING</strong> — clearing its attempt count, schedule and last error — and the
        scheduler delivers it again. Jobs in flight and jobs that already completed are left alone.
      </Message>
    </div>

    <template #footer>
      <Button label="Cancel" text severity="secondary" :disabled="running" @click="visible = false" />
      <Button
        :label="preview ? `Requeue ${plural(toRequeue, 'job')}` : 'Requeue'"
        icon="pi pi-replay"
        severity="danger"
        :loading="running"
        :disabled="!canConfirm"
        @click="confirm"
      />
    </template>
  </Dialog>
</template>

<style scoped>
.rbf {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.rbf-heading {
  margin: 0 0 0.375rem;
  font-size: 0.8125rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.02em;
  color: var(--text-color-secondary);
}

.rbf-filters {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 0.25rem 0.75rem;
  margin: 0;
  font-size: 0.875rem;
}

.rbf-filters dt {
  color: var(--text-color-secondary);
}

.rbf-filters dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.rbf-nofilter {
  margin: 0;
  font-size: 0.875rem;
  color: var(--p-orange-600, #d97706);
}

.rbf-loading {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
}

.rbf-line {
  margin: 0 0 0.5rem;
  font-size: 0.875rem;
}

.rbf-statuses {
  display: flex;
  flex-wrap: wrap;
  gap: 0.375rem;
  margin-bottom: 0.5rem;
}

.text-muted {
  color: var(--text-color-secondary);
}
</style>
