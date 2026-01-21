<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue';
import Toast from 'primevue/toast';
import ConfirmDialog from 'primevue/confirmdialog';
import GlobalToast from '@/components/GlobalToast.vue';
import PermissionDeniedModal from '@/components/PermissionDeniedModal.vue';
import { onApiError } from '@/api/client';
import { usePermissionsStore } from '@/stores/permissions';

const permissionsStore = usePermissionsStore();

let unsubscribe: (() => void) | null = null;

onMounted(() => {
  // Listen for 401/403 API errors globally
  unsubscribe = onApiError((status, message) => {
    permissionsStore.handleApiError(status, message);
  });
});

onUnmounted(() => {
  if (unsubscribe) {
    unsubscribe();
  }
});
</script>

<template>
  <RouterView />
  <Toast />
  <ConfirmDialog />
  <GlobalToast />
  <PermissionDeniedModal />
</template>
