<script setup lang="ts">
import { ref, onMounted } from 'vue';
import DataTable from 'primevue/datatable';
import Column from 'primevue/column';
import Button from 'primevue/button';
import Tag from 'primevue/tag';
import Message from 'primevue/message';
import Dialog from 'primevue/dialog';
import { getApiBffDebugEvents, getApiBffDebugEventsById } from '@/api/generated';

interface RawEvent {
  id: string;
  specVersion: string;
  type: string;
  source: string;
  subject: string;
  time: string;
  data: string;
  messageGroup?: string;
  correlationId?: string;
  causationId?: string;
  deduplicationId?: string;
  contextData?: { key: string; value: string }[];
  clientId?: string;
}

const events = ref<RawEvent[]>([]);
const loading = ref(true);
const totalRecords = ref(0);
const currentPage = ref(0);
const pageSize = ref(20);

// Detail dialog
const selectedEvent = ref<RawEvent | null>(null);
const showDetailDialog = ref(false);
const loadingDetail = ref(false);

onMounted(async () => {
  await loadEvents();
});

async function loadEvents() {
  loading.value = true;
  try {
    const response = await getApiBffDebugEvents({
      query: {
        page: currentPage.value,
        size: pageSize.value
      }
    });
    if (response.data) {
      events.value = (response.data.items || []) as RawEvent[];
      totalRecords.value = response.data.totalItems || 0;
    }
  } catch (error) {
    console.error('Failed to load raw events:', error);
  } finally {
    loading.value = false;
  }
}

async function onPage(event: { page: number; rows: number }) {
  currentPage.value = event.page;
  pageSize.value = event.rows;
  await loadEvents();
}

async function viewEventDetail(event: RawEvent) {
  loadingDetail.value = true;
  showDetailDialog.value = true;
  try {
    const response = await getApiBffDebugEventsById({ path: { id: event.id } });
    if (response.data) {
      selectedEvent.value = response.data as RawEvent;
    }
  } catch (error) {
    console.error('Failed to load event details:', error);
  } finally {
    loadingDetail.value = false;
  }
}

function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleString();
}

function formatData(data: string | undefined): string {
  if (!data) return '-';
  try {
    return JSON.stringify(JSON.parse(data), null, 2);
  } catch {
    return data;
  }
}

function truncateId(id: string): string {
  if (!id) return '-';
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
      This is a debug view of the raw <code>events</code> collection. This collection is write-optimized
      with minimal indexes. For regular queries, use the <strong>Events</strong> page which queries the
      read-optimized <code>events_read</code> projection.
    </Message>

    <div class="fc-card">
      <div class="toolbar">
        <Button
          icon="pi pi-refresh"
          text
          rounded
          @click="loadEvents"
          v-tooltip="'Refresh'"
        />
        <span class="text-muted ml-2">
          Showing raw events (no filtering - queries would be slow on this collection)
        </span>
      </div>

      <DataTable
        :value="events"
        :loading="loading"
        :lazy="true"
        :paginator="true"
        :rows="pageSize"
        :totalRecords="totalRecords"
        :rowsPerPageOptions="[10, 20, 50]"
        @page="onPage"
        stripedRows
        emptyMessage="No events found"
        tableStyle="min-width: 60rem"
      >
        <Column field="id" header="Event ID" style="width: 10rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ truncateId(data.id) }}</span>
          </template>
        </Column>
        <Column field="type" header="Type">
          <template #body="{ data }">
            <Tag :value="data.type" severity="secondary" />
          </template>
        </Column>
        <Column field="source" header="Source" />
        <Column field="subject" header="Subject">
          <template #body="{ data }">
            <span class="text-sm truncate-cell">{{ data.subject || '-' }}</span>
          </template>
        </Column>
        <Column field="deduplicationId" header="Dedup ID" style="width: 10rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ truncateId(data.deduplicationId) }}</span>
          </template>
        </Column>
        <Column field="time" header="Time" style="width: 12rem">
          <template #body="{ data }">
            <span class="text-sm">{{ formatDate(data.time) }}</span>
          </template>
        </Column>
        <Column header="Actions" style="width: 6rem">
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
    </div>

    <!-- Event Detail Dialog -->
    <Dialog
      v-model:visible="showDetailDialog"
      header="Raw Event Details"
      :style="{ width: '700px' }"
      modal
    >
      <div v-if="loadingDetail" class="flex justify-center p-4">
        <i class="pi pi-spin pi-spinner" style="font-size: 2rem"></i>
      </div>
      <div v-else-if="selectedEvent" class="event-detail">
        <div class="detail-row">
          <label>ID</label>
          <span class="font-mono">{{ selectedEvent.id }}</span>
        </div>
        <div class="detail-row">
          <label>Type</label>
          <Tag :value="selectedEvent.type" severity="secondary" />
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
  max-width: 200px;
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
