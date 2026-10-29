<script setup lang="ts">
import { toast } from "@/utils/errorBus";
import type { DefinitionExport } from "@/utils/definition-transfer";

// The exported definition JSON, read-only, with Copy and Download
// (docs/ui/definition-export-import.md). `notes` say which references were
// left out because the viewer can't read them.
const props = defineProps<{
	visible: boolean;
	definition: DefinitionExport | null;
}>();

const emit = defineEmits<{
	"update:visible": [value: boolean];
}>();

async function copy() {
	if (!props.definition) return;
	try {
		await navigator.clipboard.writeText(props.definition.text);
		toast.info("Copied", "Definition copied to clipboard");
	} catch {
		toast.error("Copy failed", "Select the text and copy it manually");
	}
}

function download() {
	if (!props.definition) return;
	const blob = new Blob([props.definition.text], { type: "application/json" });
	const url = URL.createObjectURL(blob);
	const link = document.createElement("a");
	link.href = url;
	link.download = props.definition.filename;
	link.click();
	URL.revokeObjectURL(url);
}
</script>

<template>
  <Dialog
    :visible="visible"
    @update:visible="emit('update:visible', $event)"
    header="Export JSON"
    :style="{ width: '640px' }"
    :modal="true"
  >
    <template v-if="definition">
      <Message
        v-for="note in definition.notes"
        :key="note"
        severity="warn"
        :closable="false"
        class="export-note"
      >
        {{ note }}
      </Message>
      <p class="export-hint">
        Safe values only: no ids, no credentials. References are by code, so the
        file can be imported on another environment.
      </p>
      <pre class="export-json" tabindex="0">{{ definition.text }}</pre>
    </template>

    <template #footer>
      <Button label="Close" text @click="emit('update:visible', false)" />
      <Button label="Copy" icon="pi pi-copy" outlined @click="copy" />
      <Button label="Download" icon="pi pi-download" @click="download" />
    </template>
  </Dialog>
</template>

<style scoped>
.export-note {
  margin-bottom: 8px;
}

.export-hint {
  margin: 0 0 8px 0;
  font-size: 13px;
  color: #64748b;
}

.export-json {
  margin: 0;
  padding: 12px;
  max-height: 55vh;
  overflow: auto;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  font-family: monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre;
}
</style>
