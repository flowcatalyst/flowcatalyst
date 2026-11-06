<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useListState } from "@/composables/useListState";
import { useReturnTo } from "@/composables/useReturnTo";
import {
	scheduledJobsApi,
	type ScheduledJob,
	type ScheduledJobInstance,
} from "@/api/scheduled-jobs";

const route = useRoute();
const jobId = String(route.params["id"]);
const { returnTo, forwardFrom } = useReturnTo();

const job = ref<ScheduledJob | null>(null);
const instances = ref<ScheduledJobInstance[]>([]);
const total = ref(0);
const loading = ref(false);

const { filters, page, pageSize, hasActiveFilters, clearFilters, onPage } =
	useListState(
		{
			filters: {
				status: { type: "string", key: "status" },
				triggerKind: { type: "string", key: "trigger" },
			},
			pageSize: 50,
			sortField: "firedAt",
			sortOrder: "desc",
		},
		() => load(),
	);

const STATUS_OPTIONS = [
	{ label: "Queued", value: "QUEUED" },
	{ label: "In flight", value: "IN_FLIGHT" },
	{ label: "Delivered", value: "DELIVERED" },
	{ label: "Completed", value: "COMPLETED" },
	{ label: "Failed", value: "FAILED" },
	{ label: "Delivery failed", value: "DELIVERY_FAILED" },
];
const TRIGGER_OPTIONS = [
	{ label: "Cron", value: "CRON" },
	{ label: "Manual", value: "MANUAL" },
];

async function load() {
	loading.value = true;
	try {
		const [jobResult, listResult] = await Promise.all([
			scheduledJobsApi.get(jobId),
			scheduledJobsApi.listInstances(jobId, {
				status: filters.status.value || undefined,
				triggerKind: filters.triggerKind.value || undefined,
				page: page.value,
				size: pageSize.value,
			}),
		]);
		job.value = jobResult;
		instances.value = listResult.data;
		total.value = listResult.total;
	} catch (err) {
		console.error("Failed to load instances", err);
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
function fmt(s?: string): string {
	return s ? new Date(s).toLocaleString() : "—";
}

function onRowClick(event: { data: ScheduledJobInstance }) {
	forwardFrom(`/scheduled-jobs/instances/${event.data.id}`);
}
</script>

<template>
	<div class="page-container">
		<nav class="breadcrumb" aria-label="Breadcrumb">
			<a href="#" @click.prevent="returnTo('/scheduled-jobs')">Scheduled Jobs</a>
			<template v-if="job">
				<span class="sep">/</span>
				<a href="#" @click.prevent="forwardFrom(`/scheduled-jobs/${job.id}`)">{{ job.code }}</a>
			</template>
			<span class="sep">/</span>
			<span>Firings</span>
		</nav>

		<header class="page-header">
			<div>
				<h1 class="page-title">Firings<template v-if="job">: {{ job.name }}</template></h1>
				<p class="page-subtitle">Every firing of this job, newest first</p>
			</div>
		</header>

		<div class="fc-card table-card">
			<div class="filters">
				<div class="filter">
					<label class="fc-field-label" for="firings-status">Status</label>
					<Select
						v-model="filters.status.value"
						input-id="firings-status"
						:options="STATUS_OPTIONS"
						option-label="label"
						option-value="value"
						placeholder="All statuses"
						show-clear
						class="w-full"
					/>
				</div>
				<div class="filter">
					<label class="fc-field-label" for="firings-trigger">Trigger</label>
					<Select
						v-model="filters.triggerKind.value"
						input-id="firings-trigger"
						:options="TRIGGER_OPTIONS"
						option-label="label"
						option-value="value"
						placeholder="All triggers"
						show-clear
						class="w-full"
					/>
				</div>
				<div class="filter clear">
					<Button
						v-if="hasActiveFilters"
						label="Clear"
						icon="pi pi-filter-slash"
						text
						severity="secondary"
						@click="clearFilters"
					/>
				</div>
			</div>

			<DataTable
				:value="instances"
				:loading="loading"
				:total-records="total"
				:rows="pageSize"
				:first="page * pageSize"
				lazy
				paginator
				:rows-per-page-options="[20, 50, 100, 200]"
				data-key="id"
				row-hover
				selection-mode="single"
				@row-click="onRowClick"
				@page="onPage"
			>
				<template #empty>
					<div class="empty-message">No firings found.</div>
				</template>
				<Column header="Fired At">
					<template #body="{ data }"><span class="nowrap">{{ fmt(data.firedAt) }}</span></template>
				</Column>
				<Column header="Scheduled For">
					<template #body="{ data }"><span class="nowrap">{{ fmt(data.scheduledFor) }}</span></template>
				</Column>
				<Column header="Trigger" field="triggerKind" />
				<Column header="Status">
					<template #body="{ data }">
						<Tag :value="data.status" :severity="statusSeverity(data.status)" />
					</template>
				</Column>
				<Column header="Attempts" field="deliveryAttempts" />
				<Column header="Delivered">
					<template #body="{ data }"><span class="nowrap">{{ fmt(data.deliveredAt) }}</span></template>
				</Column>
				<Column header="Completed">
					<template #body="{ data }"><span class="nowrap">{{ fmt(data.completedAt) }}</span></template>
				</Column>
				<Column header="Error" style="max-width: 240px">
					<template #body="{ data }">
						<span v-if="data.deliveryError" class="error-text" :title="data.deliveryError">
							{{ data.deliveryError }}
						</span>
					</template>
				</Column>
			</DataTable>
		</div>
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

.table-card {
	padding: 0;
	overflow: hidden;
}

.filters {
	display: grid;
	grid-template-columns: repeat(3, minmax(0, 1fr));
	gap: 12px;
	padding: 16px;
	border-bottom: 1px solid #e2e8f0;
}

@media (max-width: 768px) {
	.filters {
		grid-template-columns: minmax(0, 1fr);
	}
}

.clear {
	display: flex;
	align-items: flex-end;
}

.nowrap {
	white-space: nowrap;
}

.error-text {
	display: block;
	overflow: hidden;
	font-size: 12px;
	color: #dc2626;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.empty-message {
	padding: 24px;
	text-align: center;
	color: #64748b;
}
</style>
