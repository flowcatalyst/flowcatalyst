<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { dispatchJobsApi, type RequeueResponse } from "@/api/dispatch-jobs";
import { toast } from "@/utils/errorBus";
import RedriveConfirmStep from "./RedriveConfirmStep.vue";
import { initialRedrive, redriveReady, resetRedrive, startRedrive } from "./redriveConfirm";
import { plural } from "./requeueByFilter";

// Requeue of ticked rows when some of them are COMPLETED and the user may
// redrive: a plain requeue leaves the COMPLETED ones alone; redriving them is
// the separate, dangerous option with its own typed confirmation. Users
// without redrive-completed never reach this dialog (the list requeues
// straight away and reports what the server left alone).

const visible = defineModel<boolean>("visible", { required: true });
const props = defineProps<{
	/** Every selected job id. */
	ids: string[];
	/** How many of them are COMPLETED (by the status shown in the list). */
	completedCount: number;
}>();
const emit = defineEmits<{
	/** The run finished (result), or failed (null) - reload the list. */
	done: [result: RequeueResponse | null];
}>();

const redrive = ref(initialRedrive());
const running = ref(false);

watch(visible, (open) => {
	if (open) redrive.value = resetRedrive();
});

const otherCount = computed(() => props.ids.length - props.completedCount);
const inRedriveStep = computed(() => redrive.value.phase === "confirm");
const canRedriveNow = computed(() => !running.value && redriveReady(redrive.value, props.completedCount));

async function run(withRedrive: boolean) {
	if (running.value || (withRedrive && !canRedriveNow.value)) return;
	running.value = true;
	try {
		const res = await dispatchJobsApi.requeue(props.ids, withRedrive);
		visible.value = false;
		emit("done", res);
	} catch (error) {
		toast.error("Requeue failed", error instanceof Error ? error.message : undefined);
		emit("done", null);
	} finally {
		running.value = false;
	}
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    :header="inRedriveStep ? 'Redrive COMPLETED jobs' : 'Requeue selected jobs'"
    modal
    :closable="!running"
    :style="{ width: '520px' }"
    :breakpoints="{ '640px': '95vw' }"
  >
    <RedriveConfirmStep v-if="inRedriveStep" v-model:state="redrive" :completed-count="completedCount" :confirm-count="completedCount" />
    <div v-else class="rs">
      <p class="rs-line">
        <strong>{{ plural(ids.length, "job") }}</strong> selected, <strong>{{ completedCount.toLocaleString() }}</strong> of them
        COMPLETED. A requeue leaves COMPLETED jobs alone; jobs in flight (QUEUED or PROCESSING) are also never reset.
      </p>
      <div class="rs-redrive">
        <Button
          :label="`Also redrive ${completedCount.toLocaleString()} COMPLETED ${completedCount === 1 ? 'job' : 'jobs'}…`"
          icon="pi pi-exclamation-triangle"
          severity="danger"
          outlined
          size="small"
          :disabled="running"
          @click="redrive = startRedrive()"
        />
        <small class="text-muted">They were already delivered; this will deliver them again. You will be asked to confirm a second time.</small>
      </div>
    </div>

    <template #footer>
      <template v-if="inRedriveStep">
        <Button label="Back" text severity="secondary" :disabled="running" @click="redrive = resetRedrive()" />
        <Button
          :label="`Redrive ${plural(completedCount, 'COMPLETED job')}`"
          icon="pi pi-replay"
          severity="danger"
          :loading="running"
          :disabled="!canRedriveNow"
          @click="run(true)"
        />
      </template>
      <template v-else>
        <Button label="Cancel" text severity="secondary" :disabled="running" @click="visible = false" />
        <Button
          :label="`Requeue ${plural(otherCount, 'other job')}`"
          icon="pi pi-replay"
          severity="danger"
          :loading="running"
          :disabled="running || otherCount === 0"
          @click="run(false)"
        />
      </template>
    </template>
  </Dialog>
</template>

<style scoped>
.rs-line {
  margin: 0 0 1rem;
  font-size: 0.875rem;
}
.rs-redrive {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.375rem;
}
.text-muted {
  color: var(--text-color-secondary);
}
</style>
