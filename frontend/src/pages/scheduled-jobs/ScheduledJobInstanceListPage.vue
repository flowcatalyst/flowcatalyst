<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
	scheduledJobsApi,
	type InstanceStatus,
	type ScheduledJob,
	type ScheduledJobInstance,
	type TriggerKind,
} from "@/api/scheduled-jobs";

const route = useRoute();
const router = useRouter();
const jobId = String(route.params["id"]);

const job = ref<ScheduledJob | null>(null);
const instances = ref<ScheduledJobInstance[]>([]);
const total = ref(0);
const page = ref(0);
const size = ref(50);
const loading = ref(false);

const filterStatus = ref<InstanceStatus | "">("");
const filterTriggerKind = ref<TriggerKind | "">("");

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
				status: filterStatus.value || undefined,
				triggerKind: filterTriggerKind.value || undefined,
				page: page.value,
				size: size.value,
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

function onFilterChange() {
	page.value = 0;
	load();
}

function onPageChange(event: { page: number; rows: number }) {
	page.value = event.page;
	size.value = event.rows;
	load();
}

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
	router.push(`/scheduled-jobs/instances/${event.data.id}`);
}
</script>

<template>
	<div class="card">
		<div class="text-sm text-gray-500">
			<router-link to="/scheduled-jobs" class="hover:underline">Scheduled Jobs</router-link>
			<span v-if="job"> / <router-link :to="`/scheduled-jobs/${job.id}`" class="hover:underline">{{ job.code }}</router-link></span>
			/ Firings
		</div>
		<h2 v-if="job" class="mt-1">Firings: {{ job.name }}</h2>

		<div class="grid grid-cols-1 md:grid-cols-3 gap-3 mt-4">
			<div>
				<label class="block text-sm font-medium mb-1">Status</label>
				<Select
					v-model="filterStatus"
					:options="STATUS_OPTIONS"
					option-label="label"
					option-value="value"
					placeholder="All statuses"
					show-clear
					@change="onFilterChange"
				/>
			</div>
			<div>
				<label class="block text-sm font-medium mb-1">Trigger</label>
				<Select
					v-model="filterTriggerKind"
					:options="TRIGGER_OPTIONS"
					option-label="label"
					option-value="value"
					placeholder="All triggers"
					show-clear
					@change="onFilterChange"
				/>
			</div>
		</div>

		<DataTable
			:value="instances"
			:loading="loading"
			:total-records="total"
			:rows="size"
			:first="page * size"
			lazy
			paginator
			:rows-per-page-options="[20, 50, 100, 200]"
			data-key="id"
			row-hover
			selection-mode="single"
			class="mt-4"
			@row-click="onRowClick"
			@page="onPageChange"
		>
			<Column header="Fired At">
				<template #body="{ data }">{{ fmt(data.firedAt) }}</template>
			</Column>
			<Column header="Scheduled For">
				<template #body="{ data }">{{ fmt(data.scheduledFor) }}</template>
			</Column>
			<Column header="Trigger" field="triggerKind" />
			<Column header="Status">
				<template #body="{ data }">
					<Tag :value="data.status" :severity="statusSeverity(data.status)" />
				</template>
			</Column>
			<Column header="Attempts" field="deliveryAttempts" />
			<Column header="Delivered">
				<template #body="{ data }">{{ fmt(data.deliveredAt) }}</template>
			</Column>
			<Column header="Completed">
				<template #body="{ data }">{{ fmt(data.completedAt) }}</template>
			</Column>
			<Column header="Error" style="max-width: 200px">
				<template #body="{ data }">
					<span v-if="data.deliveryError" class="text-xs text-red-500 truncate">
						{{ data.deliveryError }}
					</span>
				</template>
			</Column>
		</DataTable>
	</div>
</template>
