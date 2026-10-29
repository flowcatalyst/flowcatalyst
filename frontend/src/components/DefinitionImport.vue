<script setup lang="ts">
import { ref } from "vue";
import type { ImportOutcome } from "@/utils/definition-transfer";

// "Import from JSON" for a create form (docs/ui/definition-export-import.md).
// It only fills the form: `run` parses the pasted/loaded text, sets the
// fields and returns what it could not apply. The notice stays visible below
// the button until the drawer closes.
const props = defineProps<{
	run: (text: string) => Promise<ImportOutcome> | ImportOutcome;
}>();

const open = ref(false);
const text = ref("");
const error = ref<string | null>(null);
const working = ref(false);
const notices = ref<string[] | null>(null);

function show() {
	text.value = "";
	error.value = null;
	open.value = true;
}

async function onFile(event: Event) {
	const input = event.target as HTMLInputElement;
	const file = input.files?.[0];
	if (!file) return;
	text.value = await file.text();
	error.value = null;
	// Allow choosing the same file again.
	input.value = "";
}

async function apply() {
	working.value = true;
	try {
		const outcome = await props.run(text.value);
		if (outcome.error) {
			error.value = outcome.error;
			return;
		}
		notices.value = outcome.notices ?? [];
		open.value = false;
	} finally {
		working.value = false;
	}
}
</script>

<template>
  <div class="definition-import">
    <Button
      label="Import from JSON"
      icon="pi pi-upload"
      severity="secondary"
      outlined
      size="small"
      @click="show"
    />

    <Message
      v-if="notices"
      :severity="notices.length ? 'warn' : 'success'"
      :closable="false"
      class="import-notice"
    >
      <template v-if="notices.length">
        <strong>Imported. Review before creating.</strong>
        <ul>
          <li v-for="notice in notices" :key="notice">{{ notice }}</li>
        </ul>
      </template>
      <template v-else>Imported. Review and create.</template>
    </Message>

    <Dialog
      v-model:visible="open"
      header="Import from JSON"
      :style="{ width: '600px' }"
      :modal="true"
    >
      <p class="import-hint">
        Paste a definition exported from another environment, or choose its file.
        This only fills in the form; nothing is saved until you create.
      </p>
      <Textarea
        v-model="text"
        rows="12"
        class="import-text"
        placeholder='{ "flowcatalyst": "definition", ... }'
        @input="error = null"
      />
      <input type="file" accept=".json,application/json" class="import-file" @change="onFile" />
      <Message v-if="error" severity="error" :closable="false" class="import-error">
        {{ error }}
      </Message>

      <template #footer>
        <Button label="Cancel" text :disabled="working" @click="open = false" />
        <Button
          label="Fill form"
          icon="pi pi-check"
          :loading="working"
          :disabled="!text.trim()"
          @click="apply"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.definition-import {
  margin-bottom: 20px;
}

.import-notice {
  margin-top: 12px;
}

.import-notice ul {
  margin: 6px 0 0 0;
  padding-left: 18px;
}

.import-hint {
  margin: 0 0 12px 0;
  font-size: 13px;
  color: #64748b;
}

.import-text {
  width: 100%;
  font-family: monospace;
  font-size: 12px;
}

.import-file {
  display: block;
  margin-top: 12px;
  font-size: 13px;
}

.import-error {
  margin-top: 12px;
}
</style>
