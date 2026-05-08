<script setup lang="ts">
import { ref } from "vue";
import { onMounted } from "vue";
import { bffFetch } from "@/api/client";
import { useCursorPagination } from "@/composables/useCursorPagination";

interface RawEvent {
	id: string;
	specVersion: string;
	eventType: string;
	source: string;
	subject: string;
	time: string;
	data: unknown;
	messageGroup?: string;
	correlationId?: string;
	causationId?: string;
	deduplicationId?: string;
	contextData?: { key: string; value: string }[];
	clientId?: string;
}

const pageSize = 20;

const cursor = useCursorPagination<RawEvent>({
	fetchPage: async (after) => {
		const qs = new URLSearchParams();
		qs.set("size", String(pageSize));
		if (after) qs.set("after", after);
		return bffFetch<{ items: RawEvent[]; hasMore: boolean; nextCursor?: string }>(
			`/debug/events?${qs}`,
		);
	},
});
const events = cursor.items;
const loading = cursor.loading;

// Detail dialog
const selectedEvent = ref<RawEvent | null>(null);
const showDetailDialog = ref(false);

onMounted(async () => {
	await cursor.loadFirst();
});

async function viewEventDetail(event: RawEvent) {
	showDetailDialog.value = true;
	try {
		const full = await bffFetch<RawEvent>(`/debug/events/${event.id}`);
		selectedEvent.value = full;
	} catch {
		selectedEvent.value = event;
	}
}

function formatDate(dateStr: string | undefined): string {
	if (!dateStr) return "-";
	return new Date(dateStr).toLocaleString();
}

function formatData(data: unknown): string {
	if (!data) return "-";
	if (typeof data === "string") {
		try {
			return JSON.stringify(JSON.parse(data), null, 2);
		} catch {
			return data;
		}
	}
	return JSON.stringify(data, null, 2);
}

function truncateId(id: string): string {
	if (!id) return "-";
	return id.length > 10 ? `${id.slice(0, 10)}...` : id;
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Raw Events</h1>
        <p class="page-subtitle">Debug view of the transactional event store</p>
      </div>
    </header>

    <Message severity="warn" :closable="false" class="mb-4">
      This is a debug view of the raw <code>events</code> collection. This collection is
      write-optimized with minimal indexes. For regular queries, use the
      <strong>Events</strong> page which queries the read-optimized
      <code>events_read</code> projection.
    </Message>

    <div class="fc-card">
      <div class="toolbar">
        <Button icon="pi pi-refresh" text rounded @click="cursor.refresh" v-tooltip="'Refresh'" />
        <span class="text-muted ml-2">
          Showing raw events (no filtering - queries would be slow on this collection)
        </span>
      </div>

      <DataTable
        :value="events"
        :loading="loading"
        stripedRows
        resizableColumns
        columnResizeMode="expand"
        emptyMessage="No events found"
        tableStyle="min-width: 60rem"
      >
        <Column field="id" header="Event ID" style="min-width: 14rem; width: 14rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ truncateId(data.id) }}</span>
          </template>
        </Column>
        <Column field="eventType" header="Type" style="min-width: 14rem">
          <template #body="{ data }">
            <Tag :value="data.eventType" severity="secondary" />
          </template>
        </Column>
        <Column field="source" header="Source" style="min-width: 10rem" />
        <Column field="subject" header="Subject" style="min-width: 10rem; max-width: 18rem">
          <template #body="{ data }">
            <span class="text-sm truncate-cell">{{ data.subject || '-' }}</span>
          </template>
        </Column>
        <Column field="deduplicationId" header="Dedup ID" style="min-width: 10rem; width: 10rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ truncateId(data.deduplicationId) }}</span>
          </template>
        </Column>
        <Column field="time" header="Time" style="min-width: 12rem; width: 12rem">
          <template #body="{ data }">
            <span class="text-sm">{{ formatDate(data.time) }}</span>
          </template>
        </Column>
        <Column header="Actions" style="width: 5rem; min-width: 5rem">
          <template #body="{ data }">
            <Button
              icon="pi pi-eye"
              text
              rounded
              v-tooltip="'View details'"
              @click="viewEventDetail(data)"
            />
          </template>
        </Column>
      </DataTable>

      <!-- Cursor pager. msg_events is unbounded; we never count. -->
      <div class="cursor-pager">
        <Button
          icon="pi pi-angle-left"
          label="Newer"
          text
          :disabled="!cursor.hasPrev.value || cursor.loading.value"
          @click="cursor.loadPrev"
        />
        <span class="page-indicator">Page {{ cursor.page.value }}</span>
        <Button
          icon="pi pi-angle-right"
          iconPos="right"
          label="Older"
          text
          :disabled="!cursor.hasMore.value || cursor.loading.value"
          @click="cursor.loadNext"
        />
      </div>
    </div>

    <!-- Event Detail Dialog -->
    <Dialog
      v-model:visible="showDetailDialog"
      header="Raw Event Details"
      :style="{ width: 'clamp(700px, 70vw, 1024px)' }"
      modal
    >
      <div v-if="selectedEvent" class="event-detail">
        <div class="detail-row">
          <label>ID</label>
          <span class="font-mono">{{ selectedEvent.id }}</span>
        </div>
        <div class="detail-row">
          <label>Type</label>
          <Tag :value="selectedEvent.eventType" severity="secondary" />
        </div>
        <div class="detail-row">
          <label>Source</label>
          <span>{{ selectedEvent.source }}</span>
        </div>
        <div class="detail-row">
          <label>Subject</label>
          <span>{{ selectedEvent.subject || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Time</label>
          <span>{{ formatDate(selectedEvent.time) }}</span>
        </div>
        <div class="detail-row">
          <label>Client ID</label>
          <span class="font-mono">{{ selectedEvent.clientId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Message Group</label>
          <span>{{ selectedEvent.messageGroup || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Correlation ID</label>
          <span class="font-mono">{{ selectedEvent.correlationId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Causation ID</label>
          <span class="font-mono">{{ selectedEvent.causationId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Deduplication ID</label>
          <span class="font-mono">{{ selectedEvent.deduplicationId || '-' }}</span>
        </div>
        <div class="detail-section">
          <label>Data</label>
          <pre class="data-block">{{ formatData(selectedEvent.data) }}</pre>
        </div>
        <div v-if="selectedEvent.contextData?.length" class="detail-section">
          <label>Context Data</label>
          <div class="context-data">
            <div v-for="cd in selectedEvent.contextData" :key="cd.key" class="context-item">
              <span class="context-key">{{ cd.key }}:</span>
              <span class="context-value">{{ cd.value }}</span>
            </div>
          </div>
        </div>
      </div>
    </Dialog>
  </div>
</template>

<style scoped>
.toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 16px;
}

.cursor-pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  padding: 0.75rem 0 0.25rem;
}

.page-indicator {
  font-size: 0.875rem;
  color: var(--text-color-secondary);
  min-width: 4.5rem;
  text-align: center;
}

.font-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.text-sm {
  font-size: 0.875rem;
}

.text-muted {
  color: var(--text-color-secondary);
  font-size: 0.875rem;
}

.truncate-cell {
  max-width: 400px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
}

.ml-2 {
  margin-left: 0.5rem;
}

.mb-4 {
  margin-bottom: 1rem;
}

.event-detail {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.detail-row {
  display: flex;
  gap: 1rem;
}

.detail-row label {
  font-weight: 600;
  min-width: 120px;
  color: var(--text-color-secondary);
}

.detail-section {
  margin-top: 0.5rem;
}

.detail-section label {
  display: block;
  font-weight: 600;
  margin-bottom: 0.5rem;
  color: var(--text-color-secondary);
}

.data-block {
  background: var(--surface-ground);
  border: 1px solid var(--surface-border);
  border-radius: 6px;
  padding: 1rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.875rem;
  overflow-x: auto;
  max-height: 300px;
  white-space: pre-wrap;
  word-break: break-word;
}

.context-data {
  background: var(--surface-ground);
  border: 1px solid var(--surface-border);
  border-radius: 6px;
  padding: 0.75rem;
}

.context-item {
  padding: 0.25rem 0;
}

.context-key {
  font-weight: 500;
  margin-right: 0.5rem;
}

.context-value {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.flex {
  display: flex;
}

.justify-center {
  justify-content: center;
}

.p-4 {
  padding: 1rem;
}
</style>
