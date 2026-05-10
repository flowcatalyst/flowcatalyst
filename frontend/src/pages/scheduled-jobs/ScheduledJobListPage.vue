<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useListState } from "@/composables/useListState";
import { useReturnTo } from "@/composables/useReturnTo";
import {
	scheduledJobsApi,
	type ScheduledJob,
	type ScheduledJobsFilterOptions,
} from "@/api/scheduled-jobs";

const router = useRouter();
const { navigateToDetail } = useReturnTo();

const jobs = ref<ScheduledJob[]>([]);
const total = ref(0);
const loading = ref(false);

const { filters, page, pageSize, hasActiveFilters, clearFilters, onPage } =
	useListState(
		{
			filters: {
				clientId: { type: "string", key: "clientId" },
				status: { type: "string", key: "status" },
				search: { type: "string", key: "q" },
			},
			pageSize: 20,
			sortField: "createdAt",
			sortOrder: "desc",
		},
		() => load(),
	);

const filterOptions = ref<ScheduledJobsFilterOptions>({
	clients: [],
	statuses: [],
});

async function loadFilterOptions() {
	try {
		filterOptions.value = await scheduledJobsApi.filterOptions();
	} catch (err) {
		console.error("Failed to load filter options", err);
	}
}

async function load() {
	loading.value = true;
	try {
		const result = await scheduledJobsApi.list({
			clientId: filters.clientId.value || undefined,
			status: filters.status.value || undefined,
			search: filters.search.value || undefined,
			page: page.value,
			size: pageSize.value,
		});
		jobs.value = result.data;
		total.value = result.total;
	} catch (err) {
		console.error("Failed to load scheduled jobs", err);
	} finally {
		loading.value = false;
	}
}

onMounted(async () => {
	await loadFilterOptions();
	await load();
});

function viewJob(job: ScheduledJob) {
	navigateToDetail(`/scheduled-jobs/${job.id}`);
}

function onRowClick(event: { data: ScheduledJob }) {
	viewJob(event.data);
}

function statusSeverity(status: string): string {
	switch (status) {
		case "ACTIVE":
			return "success";
		case "PAUSED":
			return "warn";
		case "ARCHIVED":
			return "secondary";
		default:
			return "info";
	}
}

function formatCrons(crons: string[]): string {
	if (crons.length === 0) return "—";
	if (crons.length === 1) return crons[0] ?? "";
	return `${crons[0]} (+${crons.length - 1})`;
}

function formatDate(s?: string): string {
	if (!s) return "—";
	return new Date(s).toLocaleString();
}
</script>

<template>
	<div class="card">
		<div class="flex justify-between items-center mb-4">
			<h2>Scheduled Jobs</h2>
			<Button
				label="New Scheduled Job"
				icon="pi pi-plus"
				@click="router.push('/scheduled-jobs/create')"
			/>
		</div>

		<!-- Filters -->
		<div class="grid grid-cols-1 md:grid-cols-4 gap-3 mb-4">
			<div>
				<label class="block text-sm font-medium mb-1">Client</label>
				<Select
					v-model="filters.clientId.value"
					:options="filterOptions.clients"
					option-label="label"
					option-value="value"
					placeholder="All clients"
					show-clear
				/>
			</div>
			<div>
				<label class="block text-sm font-medium mb-1">Status</label>
				<Select
					v-model="filters.status.value"
					:options="filterOptions.statuses"
					option-label="label"
					option-value="value"
					placeholder="All statuses"
					show-clear
				/>
			</div>
			<div class="md:col-span-2 flex gap-2 items-end">
				<div class="flex-1">
					<label class="block text-sm font-medium mb-1">Search</label>
					<InputText
						v-model="filters.search.value"
						placeholder="Code or name…"
						class="w-full"
					/>
				</div>
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
			:value="jobs"
			:loading="loading"
			:total-records="total"
			:rows="pageSize"
			:first="page * pageSize"
			lazy
			paginator
			:rows-per-page-options="[10, 20, 50, 100]"
			data-key="id"
			row-hover
			selection-mode="single"
			@row-click="onRowClick"
			@page="onPage"
		>
			<Column header="Code" field="code" style="width: 22%">
				<template #body="{ data }">
					<span class="font-mono text-sm">{{ data.code }}</span>
					<div v-if="data.hasActiveInstance" class="text-xs text-orange-500 mt-1">
						<i class="pi pi-spinner pi-spin mr-1" /> running
					</div>
				</template>
			</Column>
			<Column header="Name" field="name" style="width: 18%" />
			<Column header="Scope" style="width: 14%">
				<template #body="{ data }">
					<span v-if="data.clientName">{{ data.clientName }}</span>
					<span v-else class="text-gray-500 italic">Platform</span>
				</template>
			</Column>
			<Column header="Crons" style="width: 18%">
				<template #body="{ data }">
					<span class="font-mono text-xs">{{ formatCrons(data.crons) }}</span>
					<div class="text-xs text-gray-500">{{ data.timezone }}</div>
				</template>
			</Column>
			<Column header="Status" style="width: 8%">
				<template #body="{ data }">
					<Tag :value="data.status" :severity="statusSeverity(data.status)" />
				</template>
			</Column>
			<Column header="Last Fired" style="width: 14%">
				<template #body="{ data }">
					<span class="text-sm">{{ formatDate(data.lastFiredAt) }}</span>
				</template>
			</Column>
			<Column header="" style="width: 6%">
				<template #body="{ data }">
					<Button
						icon="pi pi-arrow-right"
						severity="secondary"
						text
						@click.stop="viewJob(data)"
					/>
				</template>
			</Column>
		</DataTable>
	</div>
</template>
