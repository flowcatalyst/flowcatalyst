<script setup lang="ts">
import { computed } from "vue";
import { REDRIVE_CONFIRM_WORD } from "@/api/dispatch-jobs";
import { parseTypedCount, type RedriveState } from "./redriveConfirm";
import { plural } from "./requeueByFilter";

// Second step of the redrive double confirmation: COMPLETED jobs were already
// delivered, and this delivers them again. Typing the word and the count is
// the confirmation; the parent owns the state and the submit button.
const state = defineModel<RedriveState>("state", { required: true });
const props = defineProps<{
	/** COMPLETED jobs that will be delivered again. */
	completedCount: number;
	/** The count the request will send as confirmCount (every job it resets). */
	confirmCount: number;
	/** Explain that the count includes the jobs a plain requeue would reset (the by-filter run). */
	includesPlainRequeue?: boolean;
}>();

const countWrong = computed(
	() => state.value.typedCount !== "" && parseTypedCount(state.value.typedCount) !== props.confirmCount,
);
</script>

<template>
  <div class="redrive">
    <Message severity="error" :closable="false">
      <strong>{{ plural(completedCount, "COMPLETED job") }}</strong> were already delivered. Redriving them delivers them
      <strong>again</strong>: the receiver sees each one a second time and may act on it twice. This cannot be undone.
    </Message>
    <div class="field">
      <label for="redrive-word">Type <code>{{ REDRIVE_CONFIRM_WORD }}</code> to confirm</label>
      <InputText
        id="redrive-word"
        v-model="state.typedWord"
        autocomplete="off"
        :placeholder="REDRIVE_CONFIRM_WORD"
        :invalid="state.typedWord !== '' && state.typedWord !== REDRIVE_CONFIRM_WORD"
      />
    </div>
    <div class="field">
      <label for="redrive-count">
        Type the number of jobs that will be reset: <strong>{{ confirmCount.toLocaleString() }}</strong>
      </label>
      <InputText
        id="redrive-count"
        v-model="state.typedCount"
        inputmode="numeric"
        autocomplete="off"
        :placeholder="String(confirmCount)"
        :invalid="countWrong"
      />
      <small v-if="includesPlainRequeue" class="hint">
        This is every job the run resets: the {{ plural(completedCount, "COMPLETED job") }} plus the ones a plain
        requeue would reset.
      </small>
    </div>
  </div>
</template>

<style scoped>
.redrive {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  font-size: 0.875rem;
}
.hint {
  color: var(--text-color-secondary);
}
</style>
