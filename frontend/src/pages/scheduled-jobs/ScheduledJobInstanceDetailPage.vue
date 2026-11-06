<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import {
	scheduledJobsApi,
	type ScheduledJobInstance,
	type ScheduledJobInstanceLog,
} from "@/api/scheduled-jobs";

const route = useRoute();
const instanceId = String(route.params["instanceId"]);

const inst = ref<ScheduledJobInstance | null>(null);
const logs = ref<ScheduledJobInstanceLog[]>([]);
const loading = ref(false);

async function load() {
	loading.value = true;
	try {
		const [i, l] = await Promise.all([
			scheduledJobsApi.getInstance(instanceId),
			scheduledJobsApi.listInstanceLogs(instanceId),
		]);
		inst.value = i;
		logs.value = l;
	} catch (err) {
		console.error("Failed to load instance", err);
	} finally {
		loading.value = false;
	}
}

onMounted(load);

function statusSeverity(s: string): string {
	switch (s) {
		case "DELIVERED":
		case "COMPLETED":
			return "success";
		case "QUEUED":
		case "IN_FLIGHT":
			return "info";
		case "FAILED":
		case "DELIVERY_FAILED":
			return "danger";
		default:
			return "secondary";
	}
}
function logLevelSeverity(l: string): string {
	switch (l) {
		case "ERROR":
			return "danger";
		case "WARN":
			return "warn";
		case "DEBUG":
			return "secondary";
		default:
			return "info";
	}
}
function fmt(s?: string): string {
	return s ? new Date(s).toLocaleString() : "—";
}
</script>

<template>
	<div class="page-container">
		<div v-if="loading && !inst" class="fc-card muted">Loading…</div>

		<template v-else-if="inst">
			<nav class="breadcrumb" aria-label="Breadcrumb">
				<router-link to="/scheduled-jobs">Scheduled Jobs</router-link>
				<span class="sep">/</span>
				<router-link :to="`/scheduled-jobs/${inst.scheduledJobId}`">{{ inst.jobCode }}</router-link>
				<span class="sep">/</span>
				<router-link :to="`/scheduled-jobs/${inst.scheduledJobId}/instances`">Firings</router-link>
			</nav>

			<header class="page-header">
				<div>
					<h1 class="page-title">Firing <span class="firing-id">{{ inst.id }}</span></h1>
					<div class="tags">
						<Tag :value="inst.status" :severity="statusSeverity(inst.status)" />
						<Tag :value="inst.triggerKind" severity="secondary" />
						<Tag
							v-if="inst.completionStatus"
							:value="inst.completionStatus"
							:severity="inst.completionStatus === 'SUCCESS' ? 'success' : 'danger'"
						/>
					</div>
				</div>
			</header>

			<section class="fc-card section">
				<h2 class="section-title">Details</h2>
				<div class="fc-detail-grid">
					<div>
						<span class="fc-field-label">Fired At</span>
						<div class="fc-detail-value">{{ fmt(inst.firedAt) }}</div>
					</div>
					<div>
						<span class="fc-field-label">Scheduled For</span>
						<div class="fc-detail-value">{{ fmt(inst.scheduledFor) }}</div>
					</div>
					<div>
						<span class="fc-field-label">Delivered At</span>
						<div class="fc-detail-value">{{ fmt(inst.deliveredAt) }}</div>
					</div>
					<div>
						<span class="fc-field-label">Completed At</span>
						<div class="fc-detail-value">{{ fmt(inst.completedAt) }}</div>
					</div>
					<div>
						<span class="fc-field-label">Delivery Attempts</span>
						<div class="fc-detail-value">{{ inst.deliveryAttempts }}</div>
					</div>
					<div>
						<span class="fc-field-label">Correlation Id</span>
						<div class="fc-detail-value mono">{{ inst.correlationId ?? "—" }}</div>
					</div>
					<div v-if="inst.deliveryError" class="fc-span-2">
						<span class="fc-field-label">Delivery Error</span>
						<pre class="code-block error" tabindex="0">{{ inst.deliveryError }}</pre>
					</div>
					<div v-if="inst.completionResult" class="fc-span-2">
						<span class="fc-field-label">Completion Result</span>
						<pre class="code-block" tabindex="0">{{ JSON.stringify(inst.completionResult, null, 2) }}</pre>
					</div>
				</div>
			</section>

			<section class="fc-card section logs-card">
				<h2 class="section-title logs-title">Logs</h2>
				<div v-if="logs.length === 0" class="muted empty">No log entries.</div>
				<DataTable v-else :value="logs" size="small" striped-rows>
					<Column header="Time" style="width: 18%">
						<template #body="{ data }">
							<span class="nowrap">{{ fmt(data.createdAt) }}</span>
						</template>
					</Column>
					<Column header="Level" style="width: 8%">
						<template #body="{ data }">
							<Tag :value="data.level" :severity="logLevelSeverity(data.level)" />
						</template>
					</Column>
					<Column header="Message" field="message" />
					<Column header="Metadata" style="width: 30%">
						<template #body="{ data }">
							<pre v-if="data.metadata" class="metadata">{{ JSON.stringify(data.metadata) }}</pre>
						</template>
					</Column>
				</DataTable>
			</section>
		</template>

		<div v-else class="fc-card muted">Firing not found.</div>
	</div>
</template>

<style scoped>
.breadcrumb {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 6px;
	margin-bottom: 8px;
	font-size: 13px;
	color: #627d98;
}

.breadcrumb a {
	color: #627d98;
	text-decoration: none;
}

.breadcrumb a:hover {
	color: #102a43;
	text-decoration: underline;
}

.sep {
	color: #bcccdc;
}

.firing-id {
	font-family: "SF Mono", "Consolas", monospace;
	font-size: 20px;
	font-weight: 500;
}

.tags {
	display: flex;
	flex-wrap: wrap;
	gap: 6px;
	margin-top: 8px;
}

.section {
	margin-bottom: 16px;
}

.section-title {
	margin: 0 0 12px;
	font-size: 15px;
	font-weight: 600;
	color: #102a43;
}

.mono {
	font-family: "SF Mono", "Consolas", monospace;
	font-size: 13px;
}

.code-block {
	margin: 0;
	padding: 10px 12px;
	max-height: 360px;
	overflow: auto;
	font-family: "SF Mono", "Consolas", monospace;
	font-size: 12px;
	line-height: 1.5;
	color: #1e293b;
	background: #f8fafc;
	border: 1px solid #e2e8f0;
	border-radius: 6px;
}

.code-block.error {
	color: #b91c1c;
	background: #fef2f2;
	border-color: #fecaca;
	white-space: pre-wrap;
	word-break: break-word;
}

.logs-card {
	padding: 0;
	overflow: hidden;
}

.logs-title {
	padding: 16px 16px 0;
}

.empty {
	padding: 4px 16px 16px;
}

.muted {
	color: #64748b;
}

.nowrap {
	font-size: 12px;
	white-space: nowrap;
}

.metadata {
	margin: 0;
	font-family: "SF Mono", "Consolas", monospace;
	font-size: 12px;
	white-space: pre-wrap;
	word-break: break-all;
}
</style>
