<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ApiError } from "@/api/client";
import { dispatchJobsApi, type DispatchJobDetail } from "@/api/dispatch-jobs";
import { toast } from "@/utils/errorBus";
import {
	PAYLOAD_MAX_BYTES,
	isJsonContentType,
	payloadBytes,
	payloadEditChanged,
	prettyJson,
	validatePayloadEdit,
} from "./payloadEdit";

// Edit a PENDING / FAILED job's payload. Saving changes the payload (and the
// content type, if edited) only: the job keeps its status and attempts, and the
// operator retries it afterwards with Requeue. The audit row records sizes and
// digests, not the payload.

const visible = defineModel<boolean>("visible", { required: true });
const props = defineProps<{ job: DispatchJobDetail }>();
const emit = defineEmits<{
	/** Saved (the updated job) or the job moved on and must be reloaded (null). */
	done: [job: DispatchJobDetail | null];
}>();

const text = ref("");
const contentType = ref("");
const saving = ref(false);

watch(visible, (open) => {
	if (!open) return;
	contentType.value = props.job.payloadContentType;
	// Open JSON pretty-printed so the operator edits readable text.
	const original = props.job.payload ?? "";
	text.value = (isJsonContentType(contentType.value) ? prettyJson(original) : null) ?? original;
});

const error = computed(() => validatePayloadEdit(text.value, contentType.value));
const isJson = computed(() => isJsonContentType(contentType.value));
const bytes = computed(() => payloadBytes(text.value));
// Compared against the pretty-printed form the dialog opened with, or the raw
// stored text: either way a no-op save is not offered.
const changed = computed(() => {
	const original = props.job.payload ?? "";
	const opened = (isJsonContentType(props.job.payloadContentType) ? prettyJson(original) : null) ?? original;
	return payloadEditChanged({ payload: opened, contentType: props.job.payloadContentType }, text.value, contentType.value);
});
const canSave = computed(() => !saving.value && !error.value && changed.value);

function format() {
	const pretty = prettyJson(text.value);
	if (pretty != null) text.value = pretty;
}

async function save() {
	if (!canSave.value) return;
	saving.value = true;
	try {
		const updated = await dispatchJobsApi.editPayload(props.job.id, {
			payload: text.value,
			payloadContentType: contentType.value.trim() !== props.job.payloadContentType ? contentType.value.trim() : undefined,
		});
		toast.success("Payload saved", "Requeue the job to deliver it with the new payload.");
		visible.value = false;
		emit("done", updated);
	} catch (e) {
		if (e instanceof ApiError && e.status === 409) {
			toast.error("Payload not saved", "The job is no longer PENDING or FAILED (it may be in flight). Reloading.");
			visible.value = false;
			emit("done", null);
		} else {
			toast.error("Payload not saved", e instanceof Error ? e.message : undefined);
		}
	} finally {
		saving.value = false;
	}
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    header="Edit payload"
    modal
    :closable="!saving"
    :style="{ width: '720px' }"
    :breakpoints="{ '800px': '95vw' }"
  >
    <div class="pe">
      <div class="pe-field">
        <label for="pe-ct">Content type</label>
        <InputText id="pe-ct" v-model="contentType" class="font-mono" :disabled="saving" />
      </div>
      <div class="pe-field">
        <div class="pe-head">
          <label for="pe-text">Payload</label>
          <Button
            v-if="isJson"
            label="Format JSON"
            icon="pi pi-align-left"
            text
            size="small"
            :disabled="saving"
            @click="format"
          />
        </div>
        <Textarea
          id="pe-text"
          v-model="text"
          class="pe-text"
          rows="16"
          spellcheck="false"
          :disabled="saving"
        />
        <small :class="error ? 'pe-error' : 'pe-hint'">
          {{ error ?? `${bytes.toLocaleString()} of ${PAYLOAD_MAX_BYTES.toLocaleString()} bytes` }}
        </small>
      </div>
      <Message severity="info" :closable="false">
        Saving does not retry the job — its status ({{ job.status }}) and attempts stay as they are. Use
        <strong>Requeue</strong> afterwards to deliver it with the new payload.
      </Message>
    </div>

    <template #footer>
      <Button label="Cancel" text severity="secondary" :disabled="saving" @click="visible = false" />
      <Button label="Save payload" icon="pi pi-check" :loading="saving" :disabled="!canSave" @click="save" />
    </template>
  </Dialog>
</template>

<style scoped>
.pe { display: flex; flex-direction: column; gap: 1rem; }
.pe-field { display: flex; flex-direction: column; gap: 0.375rem; }
.pe-field label { font-size: 0.8125rem; color: var(--text-color-secondary); }
.pe-head { display: flex; align-items: center; justify-content: space-between; }
.pe-text { width: 100%; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 0.8125rem; resize: vertical; }
.pe-hint { color: var(--text-color-secondary); }
.pe-error { color: var(--red-600, #dc2626); }
.font-mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
</style>
