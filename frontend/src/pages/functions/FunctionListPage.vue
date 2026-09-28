<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { functionsApi, type FunctionRecord } from "@/api/functions";
import { applicationsApi, type Application } from "@/api/applications";
import { useListState } from "@/composables/useListState";
import { useTableFilters } from "@/composables/useTableFilters";
import { useClientOptions } from "@/composables/useClientOptions";
import { useAuthStore } from "@/stores/auth";
import { userHasPermission } from "@/stores/permissions";
import ClientFilter from "@/components/ClientFilter.vue";

const router = useRouter();
const route = useRoute();
const authStore = useAuthStore();
const canManage = computed(() =>
	userHasPermission(authStore.user, "platform:function:function:manage"),
);

const functionsList = ref<FunctionRecord[]>([]);
const applications = ref<Application[]>([]);
const loading = ref(true);
const totalRecords = ref(0);

const { getLabel: getClientLabel, ensureLoaded: ensureClients } = useClientOptions();

const listState = useListState(
	{
		filters: {
			addressPrefix: { type: "string", key: "addressPrefix" },
			applicationId: { type: "string", key: "applicationId" },
			clientId: { type: "string", key: "clientId" },
		},
		pageSize: 100,
		sortField: "createdAt",
		sortOrder: "desc",
		debounceFields: ["addressPrefix"],
	},
	() => loadFunctions(),
);
const { filters, page, pageSize, onPage, onSort } = listState;

// Lazy table: the DataTable filter meta isn't bound — popup inputs write the
// listState refs directly and loadFunctions() serializes them into API params.
const { activeFilterCount, clearAll } = useTableFilters(
	listState,
	[
		{ field: "applicationId", param: "applicationId" },
		{ field: "clientId", param: "clientId" },
	],
	{ globalParam: "addressPrefix" },
);

const applicationOptions = computed(() =>
	applications.value.map((a) => ({ label: `${a.name} (${a.code})`, value: a.id })),
);

onMounted(async () => {
	await Promise.all([loadFunctions(), ensureClients(), loadApplications()]);
});

async function loadFunctions() {
	loading.value = true;
	try {
		const response = await functionsApi.list({
			addressPrefix: filters.addressPrefix.value || undefined,
			applicationId: filters.applicationId.value || undefined,
			clientId: filters.clientId.value || undefined,
			page: page.value,
			pageSize: pageSize.value,
		});
		functionsList.value = response.data;
		totalRecords.value = response.total;
	} catch (e) {
		console.error("Failed to fetch functions:", e);
	} finally {
		loading.value = false;
	}
}

async function loadApplications() {
	try {
		const response = await applicationsApi.list();
		applications.value = response.applications || [];
	} catch (e) {
		console.error("Failed to load application list", e);
	}
}

function openDetail(id: string, edit = false) {
	void router.push({
		path: `/functions/${id}`,
		query: edit ? { ...route.query, edit: "true" } : route.query,
	});
}

function openCreate() {
	void router.push({ path: "/functions/new", query: route.query });
}

function getClientScopeLabel(fn: FunctionRecord): string {
	if (fn.clientId) return getClientLabel(fn.clientId) || fn.clientId;
	return "Platform";
}

function getPoolLabel(fn: FunctionRecord): string {
	return fn.pool || "default";
}

function formatDate(dateString: string) {
	return new Date(dateString).toLocaleDateString();
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div>
        <h1 class="page-title">Functions</h1>
        <p class="page-subtitle">WebAssembly functions the platform stores, versions and runs</p>
      </div>
      <Button v-if="canManage" label="Create Function" icon="pi pi-plus" @click="openCreate" />
    </header>

    <div class="fc-card table-card">
      <DataTable
        :value="functionsList"
        :loading="loading"
        :paginator="true"
        :first="page * pageSize"
        :rows="pageSize"
        :totalRecords="totalRecords"
        :rowsPerPageOptions="[50, 100, 250, 500]"
        :lazy="true"
        :showCurrentPageReport="true"
        currentPageReportTemplate="Showing {first} to {last} of {totalRecords} functions"
        stripedRows
        rowHover
        :rowClass="() => 'clickable-row'"
        @page="onPage"
        @sort="onSort"
        @row-click="(e) => openDetail(e.data.id)"
      >
        <template #header>
          <FcTableToolbar
            v-model:search="filters.addressPrefix.value"
            search-placeholder="Search by address prefix..."
            :active-filter-count="activeFilterCount"
            :has-active-filters="listState.hasActiveFilters.value"
            @clear-all="clearAll"
          >
            <template #filters>
              <FcFormField label="Application">
                <template #default="{ id: fieldId }">
                  <Select
                    :id="fieldId"
                    v-model="filters.applicationId.value"
                    :options="applicationOptions"
                    optionLabel="label"
                    optionValue="value"
                    placeholder="All applications"
                    showClear
                    appendTo="self"
                  />
                </template>
              </FcFormField>
              <FcFormField label="Client">
                <ClientFilter
                  v-model="filters.clientId.value"
                  :multiple="false"
                  appendTo="self"
                />
              </FcFormField>
            </template>
          </FcTableToolbar>
        </template>
        <template #empty>No functions found</template>

        <Column field="address" header="Address" sortable>
          <template #body="{ data }">
            <code class="fn-address">{{ data.address }}</code>
          </template>
        </Column>
        <Column field="applicationCode" header="Application" sortable>
          <template #body="{ data }">
            <code class="app-code">{{ data.applicationCode }}</code>
          </template>
        </Column>
        <Column header="Client" sortable>
          <template #body="{ data }">
            <span class="client-scope">{{ getClientScopeLabel(data) }}</span>
          </template>
        </Column>
        <Column header="Pool" sortable>
          <template #body="{ data }">
            <code class="pool-code">{{ getPoolLabel(data) }}</code>
          </template>
        </Column>
        <Column field="warm" header="Warm">
          <template #body="{ data }">
            <Tag
              :value="data.warm ? 'Warm' : 'Lazy'"
              :severity="data.warm ? 'success' : 'secondary'"
            />
          </template>
        </Column>
        <Column field="createdAt" header="Created" sortable>
          <template #body="{ data }">
            {{ formatDate(data.createdAt) }}
          </template>
        </Column>
        <Column v-if="canManage" header="Actions" style="width: 80px">
          <template #body="{ data }">
            <Button
              v-tooltip="'Edit'"
              icon="pi pi-pencil"
              text
              rounded
              @click.stop="openDetail(data.id, true)"
            />
          </template>
        </Column>
      </DataTable>
    </div>

    <!-- Drawer outlet: detail/create child routes render over this list -->
    <RouterView v-slot="{ Component }">
      <component :is="Component" @changed="loadFunctions" />
    </RouterView>
  </div>
</template>

<style scoped>
.table-card {
  padding: 0;
  overflow: hidden;
}

.fn-address {
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 13px;
}

.app-code {
  background: #fef3c7;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  color: #92400e;
}

.pool-code {
  background: #e0f2fe;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  color: #0369a1;
}

.client-scope {
  font-size: 13px;
  color: #475569;
}
</style>
