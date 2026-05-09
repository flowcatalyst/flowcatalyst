<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { toast } from "@/utils/errorBus";
import {
	scheduledJobsApi,
	type CreateScheduledJobBody,
	type FilterOption,
} from "@/api/scheduled-jobs";

const router = useRouter();

const form = ref<CreateScheduledJobBody>({
	code: "",
	name: "",
	description: "",
	clientId: undefined,
	crons: [""],
	timezone: "UTC",
	concurrent: false,
	tracksCompletion: false,
	timeoutSeconds: undefined,
	deliveryMaxAttempts: 3,
	targetUrl: "",
});

const payloadJson = ref<string>("");
const submitting = ref(false);
const clientOptions = ref<FilterOption[]>([]);

onMounted(async () => {
	try {
		const opts = await scheduledJobsApi.filterOptions();
		clientOptions.value = opts.clients;
	} catch (err) {
		console.error("Failed to load client list", err);
	}
});

function addCron() {
	form.value.crons.push("");
}
function removeCron(idx: number) {
	if (form.value.crons.length > 1) form.value.crons.splice(idx, 1);
}

async function submit() {
	if (!form.value.code.trim() || !form.value.name.trim()) {
		toast.warn("Missing fields", "Code and name are required");
		return;
	}
	const cleanCrons = form.value.crons.map((c) => c.trim()).filter(Boolean);
	if (cleanCrons.length === 0) {
		toast.warn("Missing cron", "At least one cron expression is required");
		return;
	}

	let parsedPayload: unknown = undefined;
	if (payloadJson.value.trim()) {
		try {
			parsedPayload = JSON.parse(payloadJson.value);
		} catch {
			toast.warn("Invalid payload", "Payload must be valid JSON or empty");
			return;
		}
	}

	const body: CreateScheduledJobBody = {
		...form.value,
		crons: cleanCrons,
		clientId:
			form.value.clientId === "platform" || !form.value.clientId
				? null
				: form.value.clientId,
		payload: parsedPayload,
		description: form.value.description?.trim() || undefined,
		targetUrl: form.value.targetUrl?.trim() || undefined,
	};

	submitting.value = true;
	try {
		const result = await scheduledJobsApi.create(body);
		toast.success("Created", `Scheduled job '${form.value.code}' created`);
		router.push(`/scheduled-jobs/${result.id}`);
	} catch (err) {
		console.error("Failed to create scheduled job", err);
	} finally {
		submitting.value = false;
	}
}
</script>

<template>
	<div class="card max-w-3xl">
		<h2>New Scheduled Job</h2>

		<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4">
			<div>
				<label class="block text-sm font-medium mb-1">Code <span class="text-red-500">*</span></label>
				<InputText v-model="form.code" placeholder="daily-cleanup" class="w-full" />
				<small class="text-gray-500">Routing key the SDK uses to find the registered handler.</small>
			</div>
			<div>
				<label class="block text-sm font-medium mb-1">Name <span class="text-red-500">*</span></label>
				<InputText v-model="form.name" placeholder="Daily cleanup job" class="w-full" />
			</div>
			<div class="md:col-span-2">
				<label class="block text-sm font-medium mb-1">Description</label>
				<Textarea v-model="form.description" rows="2" class="w-full" />
			</div>
			<div>
				<label class="block text-sm font-medium mb-1">Scope</label>
				<Select
					v-model="form.clientId"
					:options="[{ value: 'platform', label: 'Platform-scoped (anchor only)' }, ...clientOptions]"
					option-label="label"
					option-value="value"
					placeholder="Select a client or platform"
				/>
			</div>
			<div>
				<label class="block text-sm font-medium mb-1">Timezone</label>
				<InputText v-model="form.timezone" placeholder="UTC" class="w-full" />
				<small class="text-gray-500">IANA name (e.g. <code>Europe/London</code>).</small>
			</div>

			<div class="md:col-span-2">
				<label class="block text-sm font-medium mb-1">
					Cron Expressions <span class="text-red-500">*</span>
				</label>
				<div v-for="(_, idx) in form.crons" :key="idx" class="flex gap-2 mb-2">
					<InputText v-model="form.crons[idx]" placeholder="0 0 * * * *" class="flex-1 font-mono" />
					<Button
						icon="pi pi-trash"
						severity="danger"
						text
						:disabled="form.crons.length === 1"
						@click="removeCron(idx)"
					/>
				</div>
				<Button label="Add another" icon="pi pi-plus" text size="small" @click="addCron" />
				<small class="block text-gray-500 mt-1">
					6-field cron syntax (sec min hour day month dow). Multiple expressions
					are unioned at fire time.
				</small>
			</div>

			<div class="md:col-span-2">
				<label class="block text-sm font-medium mb-1">Target URL</label>
				<InputText v-model="form.targetUrl" placeholder="https://app.example.com/_fc/scheduled-jobs/process" class="w-full" />
				<small class="text-gray-500">
					The platform POSTs the firing envelope here. Without it, instances
					will be marked DELIVERY_FAILED on every fire.
				</small>
			</div>

			<div>
				<label class="block text-sm font-medium mb-1">Delivery Max Attempts</label>
				<InputNumber v-model="form.deliveryMaxAttempts" :min="1" :max="20" />
			</div>
			<div>
				<label class="block text-sm font-medium mb-1">Timeout Seconds (hint)</label>
				<InputNumber v-model="form.timeoutSeconds" :min="1" placeholder="—" />
				<small class="text-gray-500">Hint passed to the SDK; not enforced by the platform.</small>
			</div>

			<div class="flex items-center gap-2">
				<Checkbox v-model="form.concurrent" :binary="true" input-id="concurrent" />
				<label for="concurrent">Allow concurrent firings</label>
			</div>
			<div class="flex items-center gap-2">
				<Checkbox v-model="form.tracksCompletion" :binary="true" input-id="tracksCompletion" />
				<label for="tracksCompletion">SDK reports completion</label>
			</div>

			<div class="md:col-span-2">
				<label class="block text-sm font-medium mb-1">Payload (JSON)</label>
				<Textarea v-model="payloadJson" rows="5" class="w-full font-mono" placeholder="{}" />
				<small class="text-gray-500">
					Optional. Passed verbatim in every firing envelope.
				</small>
			</div>
		</div>

		<div class="flex gap-2 mt-6">
			<Button label="Create" icon="pi pi-check" :loading="submitting" @click="submit" />
			<Button label="Cancel" severity="secondary" text @click="router.back()" />
		</div>
	</div>
</template>
