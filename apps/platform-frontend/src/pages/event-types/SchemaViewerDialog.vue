<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useToast } from "primevue/usetoast";
import type { SpecVersion } from "@/api/event-types";
import { generateExample } from "@/utils/schema-example";

const props = defineProps<{
	visible: boolean;
	specVersion: SpecVersion | null;
	eventCode: string;
}>();

const emit = defineEmits<{
	(e: "update:visible", value: boolean): void;
}>();

const toast = useToast();
const activeTab = ref<"schema" | "example">("schema");

const parsedSchema = computed(() => {
	if (!props.specVersion?.schema) return null;
	try {
		return JSON.parse(props.specVersion.schema);
	} catch {
		return null;
	}
});

const formattedSchema = computed(() => {
	if (!parsedSchema.value) return props.specVersion?.schema ?? "";
	return JSON.stringify(parsedSchema.value, null, 2);
});

const formattedExample = computed(() => {
	if (!parsedSchema.value) return "// Unable to generate example";
	try {
		return JSON.stringify(generateExample(parsedSchema.value), null, 2);
	} catch {
		return "// Unable to generate example";
	}
});

const displayContent = computed(() =>
	activeTab.value === "schema" ? formattedSchema.value : formattedExample.value,
);

function highlightJson(json: string): string {
	return json.replace(
		/("(?:\\.|[^"\\])*")\s*(:)|("(?:\\.|[^"\\])*")|(true|false|null)|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
		(match, key, colon, str, keyword, num) => {
			if (key) return `<span class="json-key">${key}</span>${colon}`;
			if (str) return `<span class="json-str">${str}</span>`;
			if (keyword) return `<span class="json-kw">${keyword}</span>`;
			if (num) return `<span class="json-num">${num}</span>`;
			return match;
		},
	);
}

const highlightedContent = computed(() => highlightJson(displayContent.value));

watch(
	() => props.visible,
	(v) => {
		if (v) activeTab.value = "schema";
	},
);

function close() {
	emit("update:visible", false);
}

async function copyToClipboard() {
	try {
		await navigator.clipboard.writeText(displayContent.value);
		toast.add({
			severity: "success",
			summary: "Copied",
			detail: `${activeTab.value === "schema" ? "Schema" : "Example"} copied to clipboard`,
			life: 2000,
		});
	} catch {
		toast.add({
			severity: "error",
			summary: "Error",
			detail: "Failed to copy",
			life: 2000,
		});
	}
}
</script>

<template>
  <Dialog
    :visible="visible"
    @update:visible="close"
    modal
    :header="`${eventCode} — v${specVersion?.version ?? ''}`"
    :style="{ width: '720px', maxHeight: '85vh' }"
    :contentStyle="{ padding: 0 }"
  >
    <div class="viewer-toolbar">
      <div class="tab-group">
        <button
          class="tab-btn"
          :class="{ active: activeTab === 'schema' }"
          @click="activeTab = 'schema'"
        >
          Schema
        </button>
        <button
          class="tab-btn"
          :class="{ active: activeTab === 'example' }"
          @click="activeTab = 'example'"
        >
          Example
        </button>
      </div>
      <div class="toolbar-actions">
        <Tag
          v-if="specVersion"
          :value="specVersion.status"
          :severity="specVersion.status === 'CURRENT' ? 'success' : specVersion.status === 'FINALISING' ? 'info' : 'warn'"
          class="status-tag"
        />
        <Button
          icon="pi pi-copy"
          text
          rounded
          severity="secondary"
          v-tooltip="'Copy'"
          @click="copyToClipboard"
        />
      </div>
    </div>
    <div class="code-container">
      <pre class="code-block"><code v-html="highlightedContent"></code></pre>
    </div>
  </Dialog>
</template>

<style scoped>
.viewer-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 16px;
  border-bottom: 1px solid #e2e8f0;
  background: #f8fafc;
}

.tab-group {
  display: flex;
  gap: 2px;
  background: #e2e8f0;
  border-radius: 6px;
  padding: 2px;
}

.tab-btn {
  padding: 6px 16px;
  border: none;
  background: transparent;
  border-radius: 4px;
  font-size: 13px;
  font-weight: 500;
  color: #64748b;
  cursor: pointer;
  transition: all 0.15s;
}

.tab-btn:hover {
  color: #334155;
}

.tab-btn.active {
  background: white;
  color: #0f172a;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.code-container {
  overflow: auto;
  max-height: calc(85vh - 130px);
}

.code-block {
  margin: 0;
  padding: 16px;
  background: #1e293b;
  color: #e2e8f0;
  font-family: "SF Mono", "Fira Code", "Cascadia Code", monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre;
  overflow-x: auto;
  min-height: 200px;
}

.code-block code {
  background: none;
  padding: 0;
  color: inherit;
}

.code-block :deep(.json-key) { color: #7dd3fc; }
.code-block :deep(.json-str) { color: #86efac; }
.code-block :deep(.json-num) { color: #fde68a; }
.code-block :deep(.json-kw)  { color: #c4b5fd; }
</style>
