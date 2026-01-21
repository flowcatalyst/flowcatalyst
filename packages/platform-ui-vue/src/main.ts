import { createApp } from 'vue';
import { createPinia } from 'pinia';
import PrimeVue from 'primevue/config';
import Aura from '@primevue/themes/aura';
import ConfirmationService from 'primevue/confirmationservice';
import ToastService from 'primevue/toastservice';
import Tooltip from 'primevue/tooltip';

import App from './App.vue';
import router from './router';
import { client } from './api/generated/client.gen';
import { setupApiInterceptors } from './api/interceptors';

import 'primeicons/primeicons.css';
import './styles/main.css';

// Configure API client to use relative URLs (proxied by Vite in dev)
client.setConfig({
  baseUrl: '',
  credentials: 'include',
});

// Setup global error handling and retry logic
setupApiInterceptors();

const app = createApp(App);

app.use(createPinia());
app.use(router);

app.use(PrimeVue, {
  theme: {
    preset: Aura,
    options: {
      darkModeSelector: '.dark-mode',
    },
  },
});
app.use(ConfirmationService);
app.use(ToastService);
app.directive('tooltip', Tooltip);

app.mount('#app');
