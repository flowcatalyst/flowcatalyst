<script setup lang="ts">
import { ref, onMounted } from "vue";
import { bffFetch } from "@/api/client";
import { useCursorPagination } from "@/composables/useCursorPagination";

interface RawDispatchJob {
	id: string;
	externalId?: string;
	source: string;
	kind: string;
	code: string;
	subject?: string;
	eventId?: string;
	correlationId?: string;
	targetUrl: string;
	protocol: string;
	clientId?: string;
	subscriptionId?: string;
	serviceAccountId?: string;
	dispatchPoolId?: string;
	messageGroup?: string;
	mode: string;
	sequence: number;
	status: string;
	attemptCount: number;
	maxRetries: number;
	lastError?: string;
	timeoutSeconds: number;
	retryStrategy: string;
	idempotencyKey?: string;
	createdAt: string;
	updatedAt: string;
	scheduledFor?: string;
	completedAt?: string;
	payloadContentType?: string;
	payloadLength: number;
	attemptHistoryCount: number;
}

const pageSize = 20;

const cursor = useCursorPagination<RawDispatchJob>({
	fetchPage: async (after) => {
		const qs = new URLSearchParams();
		qs.set("size", String(pageSize));
		if (after) qs.set("after", after);
		return bffFetch<{ items: RawDispatchJob[]; hasMore: boolean; nextCursor?: string }>(
			`/debug/dispatch-jobs?${qs}`,
		);
	},
});
const dispatchJobs = cursor.items;
const loading = cursor.loading;

// Detail dialog
const selectedJob = ref<RawDispatchJob | null>(null);
const showDetailDialog = ref(false);

onMounted(async () => {
	await cursor.loadFirst();
});

async function viewJobDetail(job: RawDispatchJob) {
	selectedJob.value = job;
	showDetailDialog.value = true;
}

function getSeverity(
	status: string,
):
	| "success"
	| "info"
	| "warn"
	| "danger"
	| "secondary"
	| "contrast"
	| undefined {
	switch (status) {
		case "COMPLETED":
			return "success";
		case "PENDING":
			return "info";
		case "QUEUED":
			return "info";
		case "PROCESSING":
			return "warn";
		case "FAILED":
			return "danger";
		case "CANCELLED":
			return "secondary";
		case "EXPIRED":
			return "secondary";
		default:
			return "secondary";
	}
}

function formatDate(dateStr: string | undefined): string {
	if (!dateStr) return "-";
	return new Date(dateStr).toLocaleString();
}

function truncateId(id: string | undefined): string {
	if (!id) return "-";
	return id.length > 10 ? `${id.slice(0, 10)}...` : id;
}

function formatAttempts(job: RawDispatchJob): string {
	return `${job.attemptCount || 0}/${job.maxRetries || 3}`;
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Raw Dispatch Jobs</h1>
        <p class="page-subtitle">Debug view of the transactional dispatch job store</p>
      </div>
    </header>

    <Message severity="warn" :closable="false" class="mb-4">
      This is a debug view of the raw <code>dispatch_jobs</code> collection. This collection is
      write-optimized with minimal indexes. For regular queries, use the
      <strong>Dispatch Jobs</strong> page which queries the read-optimized
      <code>dispatch_jobs_read</code> projection.
    </Message>

    <div class="fc-card">
      <div class="toolbar">
        <Button icon="pi pi-refresh" text rounded @click="cursor.refresh" v-tooltip="'Refresh'" />
        <span class="text-muted ml-2">
          Showing raw dispatch jobs (no filtering - queries would be slow on this collection)
        </span>
      </div>

      <DataTable
        :value="dispatchJobs"
        :loading="loading"
        stripedRows
        emptyMessage="No dispatch jobs found"
        tableStyle="min-width: 60rem"
      >
        <Column field="id" header="Job ID" style="width: 10rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ truncateId(data.id) }}</span>
          </template>
        </Column>
        <Column field="code" header="Code">
          <template #body="{ data }">
            <Tag :value="data.code" severity="secondary" />
          </template>
        </Column>
        <Column field="kind" header="Kind" style="width: 6rem">
          <template #body="{ data }">
            <span class="text-sm">{{ data.kind }}</span>
          </template>
        </Column>
        <Column field="status" header="Status" style="width: 8rem">
          <template #body="{ data }">
            <Tag :value="data.status" :severity="getSeverity(data.status)" />
          </template>
        </Column>
        <Column header="Attempts" style="width: 6rem">
          <template #body="{ data }">
            {{ formatAttempts(data) }}
          </template>
        </Column>
        <Column field="payloadLength" header="Payload" style="width: 6rem">
          <template #body="{ data }">
            <span class="text-sm">{{ data.payloadLength }} bytes</span>
          </template>
        </Column>
        <Column field="idempotencyKey" header="Idempotency Key" style="width: 10rem">
          <template #body="{ data }">
            <span class="font-mono text-sm">{{ truncateId(data.idempotencyKey) }}</span>
          </template>
        </Column>
        <Column field="createdAt" header="Created" style="width: 12rem">
          <template #body="{ data }">
            <span class="text-sm">{{ formatDate(data.createdAt) }}</span>
          </template>
        </Column>
        <Column header="Actions" style="width: 6rem">
          <template #body="{ data }">
            <Button
              icon="pi pi-eye"
              text
              rounded
              v-tooltip="'View details'"
              @click="viewJobDetail(data)"
            />
          </template>
        </Column>
      </DataTable>

      <!-- Cursor pager. msg_dispatch_jobs is unbounded; we never count. -->
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

    <!-- Job Detail Dialog -->
    <Dialog
      v-model:visible="showDetailDialog"
      header="Raw Dispatch Job Details"
      :style="{ width: '700px' }"
      modal
    >
      <div v-if="selectedJob" class="job-detail">
        <div class="detail-row">
          <label>ID</label>
          <span class="font-mono">{{ selectedJob.id }}</span>
        </div>
        <div class="detail-row">
          <label>Code</label>
          <Tag :value="selectedJob.code" severity="secondary" />
        </div>
        <div class="detail-row">
          <label>Kind</label>
          <span>{{ selectedJob.kind }}</span>
        </div>
        <div class="detail-row">
          <label>Status</label>
          <Tag :value="selectedJob.status" :severity="getSeverity(selectedJob.status)" />
        </div>
        <div class="detail-row">
          <label>Source</label>
          <span>{{ selectedJob.source }}</span>
        </div>
        <div class="detail-row">
          <label>Target URL</label>
          <span class="font-mono text-sm">{{ selectedJob.targetUrl }}</span>
        </div>
        <div class="detail-row">
          <label>Client ID</label>
          <span class="font-mono">{{ selectedJob.clientId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Subscription ID</label>
          <span class="font-mono">{{ selectedJob.subscriptionId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Message Group</label>
          <span>{{ selectedJob.messageGroup || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Attempts</label>
          <span
            >{{ formatAttempts(selectedJob) }} ({{ selectedJob.attemptHistoryCount }} in
            history)</span
          >
        </div>
        <div class="detail-row">
          <label>Last Error</label>
          <span class="text-danger">{{ selectedJob.lastError || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Idempotency Key</label>
          <span class="font-mono">{{ selectedJob.idempotencyKey || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Correlation ID</label>
          <span class="font-mono">{{ selectedJob.correlationId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Event ID</label>
          <span class="font-mono">{{ selectedJob.eventId || '-' }}</span>
        </div>
        <div class="detail-row">
          <label>Payload</label>
          <span>{{ selectedJob.payloadLength }} bytes ({{ selectedJob.payloadContentType }})</span>
        </div>
        <div class="detail-row">
          <label>Created At</label>
          <span>{{ formatDate(selectedJob.createdAt) }}</span>
        </div>
        <div class="detail-row">
          <label>Updated At</label>
          <span>{{ formatDate(selectedJob.updatedAt) }}</span>
        </div>
        <div class="detail-row">
          <label>Scheduled For</label>
          <span>{{ formatDate(selectedJob.scheduledFor) }}</span>
        </div>
        <div class="detail-row">
          <label>Completed At</label>
          <span>{{ formatDate(selectedJob.completedAt) }}</span>
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

.text-danger {
  color: var(--red-500);
}

.ml-2 {
  margin-left: 0.5rem;
}

.mb-4 {
  margin-bottom: 1rem;
}

.job-detail {
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
