<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useForm, useField } from 'vee-validate';
import { toTypedSchema } from '@vee-validate/zod';
import { z } from 'zod';
import { useToast } from 'primevue/usetoast';
import InputText from 'primevue/inputtext';
import Password from 'primevue/password';
import Button from 'primevue/button';
import Checkbox from 'primevue/checkbox';
import { useAuthStore } from '@/stores/auth';
import { useLoginThemeStore } from '@/stores/loginTheme';
import { checkEmailDomain, login } from '@/api/auth';

type LoginStep = 'email' | 'password' | 'redirecting';

const authStore = useAuthStore();
const themeStore = useLoginThemeStore();
const toast = useToast();

// Load theme on mount
onMounted(async () => {
  await themeStore.loadTheme();
  themeStore.applyThemeColors();
});

const step = ref<LoginStep>('email');
const isSubmitting = ref(false);
const rememberMe = ref(false);

// Email step schema
const emailSchema = toTypedSchema(
  z.object({
    email: z.string().min(1, 'Email is required').email('Please enter a valid email address'),
  }),
);

// Email form
const {
  handleSubmit: handleEmailSubmit,
  values: _emailValues,
  meta: _emailMeta,
} = useForm({
  validationSchema: emailSchema,
  initialValues: { email: '' },
});

const { value: emailValue, errorMessage: emailError } = useField<string>('email');

// Password form - separate form context
const passwordValue = ref('');
const passwordTouched = ref(false);

const isEmailValid = computed(() => {
  // Simple validation - has content and looks like an email
  const email = emailValue.value || '';
  return email.length > 0 && email.includes('@') && email.includes('.');
});

const isPasswordValid = computed(() => {
  return passwordValue.value.length > 0;
});

const currentEmail = computed(() => emailValue.value || '');

function onChangeEmail() {
  step.value = 'email';
  passwordValue.value = '';
  passwordTouched.value = false;
  authStore.setError(null);
}

const onCheckEmail = handleEmailSubmit(async (values) => {
  isSubmitting.value = true;
  authStore.setError(null);

  try {
    const result = await checkEmailDomain(values.email);

    if (result.authMethod === 'external' && result.loginUrl) {
      step.value = 'redirecting';

      // Forward OAuth params to OIDC login if this is part of an OAuth flow
      const currentParams = new URLSearchParams(window.location.search);
      let redirectUrl = result.loginUrl;

      if (currentParams.get('oauth') === 'true') {
        const oauthFields = [
          'client_id',
          'redirect_uri',
          'scope',
          'state',
          'code_challenge',
          'code_challenge_method',
          'nonce',
        ];
        const loginUrl = new URL(result.loginUrl, window.location.origin);

        for (const field of oauthFields) {
          const value = currentParams.get(field);
          if (value) {
            // Map to oauth_ prefix expected by /auth/oidc/login
            loginUrl.searchParams.set('oauth_' + field, value);
          }
        }
        redirectUrl = loginUrl.toString();
      }

      window.location.href = redirectUrl;
    } else {
      step.value = 'password';
    }
  } catch (error: any) {
    const message = error?.message || 'Unable to verify email domain. Please try again.';
    toast.add({
      severity: 'error',
      summary: 'Connection Error',
      detail: message,
      life: 5000,
    });
  } finally {
    isSubmitting.value = false;
  }
});

async function onSubmitPassword() {
  if (!isPasswordValid.value || isSubmitting.value) return;

  isSubmitting.value = true;

  try {
    await login({ email: currentEmail.value, password: passwordValue.value });
  } catch {
    // Error is handled by AuthStore
  } finally {
    isSubmitting.value = false;
  }
}
</script>

<template>
  <div class="login-container" :style="{ background: themeStore.background }">
    <div class="login-content">
      <!-- Logo and branding -->
      <div class="login-header">
        <!-- Custom logo URL -->
        <img
          v-if="themeStore.theme.logoUrl"
          :src="themeStore.theme.logoUrl"
          class="logo-image"
          alt="Logo"
        />
        <!-- Custom logo SVG -->
        <div
          v-else-if="themeStore.theme.logoSvg"
          class="logo-svg"
          v-html="themeStore.theme.logoSvg"
        />
        <!-- Default logo -->
        <div v-else class="logo-container">
          <svg class="logo-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M13 10V3L4 14h7v7l9-11h-7z"
            />
          </svg>
        </div>
        <h1 class="brand-name">{{ themeStore.theme.brandName }}</h1>
        <p class="brand-subtitle">{{ themeStore.theme.brandSubtitle }}</p>
      </div>

      <!-- Login card -->
      <div class="login-card">
        <h2 class="login-title">
          {{
            step === 'email'
              ? 'Sign in to your account'
              : step === 'password'
                ? 'Enter your password'
                : 'Redirecting...'
          }}
        </h2>

        <!-- Error message -->
        <div v-if="authStore.error" class="error-message">
          <p>{{ authStore.error }}</p>
        </div>

        <!-- Redirecting state -->
        <div v-if="step === 'redirecting'" class="redirecting-state">
          <div class="spinner"></div>
          <p>Redirecting to your organization's login...</p>
        </div>

        <!-- Email step -->
        <form v-if="step === 'email'" class="login-form" @submit.prevent="onCheckEmail">
          <div class="form-field">
            <label for="email">Email address</label>
            <InputText
              id="email"
              v-model="emailValue"
              type="email"
              placeholder="you@company.com"
              :disabled="isSubmitting"
              :invalid="!!emailError"
              class="w-full"
            />
            <small v-if="emailError" class="field-error">{{ emailError }}</small>
            <small v-else class="field-hint"
              >We'll check if your organization uses single sign-on</small
            >
          </div>

          <Button
            type="submit"
            label="Continue"
            :loading="isSubmitting"
            :disabled="!isEmailValid"
            class="w-full"
          />
        </form>

        <!-- Password step -->
        <form v-if="step === 'password'" class="login-form" @submit.prevent="onSubmitPassword">
          <!-- Show email (read-only) with change option -->
          <div class="email-display">
            <div class="email-info">
              <div class="email-avatar">
                {{ currentEmail.charAt(0).toUpperCase() }}
              </div>
              <span class="email-text">{{ currentEmail }}</span>
            </div>
            <button type="button" class="change-email-btn" @click="onChangeEmail">Change</button>
          </div>

          <div class="form-field">
            <label for="password">Password</label>
            <Password
              id="password"
              v-model="passwordValue"
              placeholder="Enter your password"
              :disabled="isSubmitting"
              :feedback="false"
              toggleMask
              inputClass="w-full"
              class="w-full"
              @blur="passwordTouched = true"
            />
          </div>

          <div class="form-options">
            <label class="remember-me">
              <Checkbox v-model="rememberMe" :binary="true" />
              <span>Remember me</span>
            </label>
            <a href="#" class="forgot-password">Forgot password?</a>
          </div>

          <Button
            type="submit"
            label="Sign in"
            :loading="isSubmitting"
            :disabled="!isPasswordValid"
            class="w-full"
          />
        </form>
      </div>

      <!-- Footer -->
      <p class="login-footer">
        {{ themeStore.theme.footerText }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--login-bg, linear-gradient(135deg, #102a43 0%, #0a1929 100%));
  padding: 16px;
}

.login-content {
  width: 100%;
  max-width: 480px;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.logo-container {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 72px;
  height: 72px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 16px;
  margin-bottom: 16px;
}

.logo-icon {
  width: 40px;
  height: 40px;
  color: white;
}

.logo-image {
  max-width: 200px;
  max-height: 72px;
  margin-bottom: 16px;
  object-fit: contain;
}

.logo-svg {
  margin-bottom: 16px;
}

.logo-svg :deep(svg) {
  max-width: 200px;
  max-height: 72px;
}

.brand-name {
  font-size: 32px;
  font-weight: 700;
  color: white;
  margin: 0;
}

.brand-subtitle {
  color: #9fb3c8;
  margin: 8px 0 0;
  font-size: 16px;
}

.login-card {
  background: white;
  border-radius: 16px;
  padding: 40px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

.login-title {
  font-size: 20px;
  font-weight: 600;
  color: #102a43;
  margin: 0 0 24px;
}

.error-message {
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 24px;
}

.error-message p {
  margin: 0;
  color: #dc2626;
  font-size: 14px;
}

.redirecting-state {
  text-align: center;
  padding: 32px 0;
}

.spinner {
  width: 32px;
  height: 32px;
  border: 4px solid #e2e8f0;
  border-top-color: var(--login-accent, #0967d2);
  border-radius: 50%;
  animation: spin 1s linear infinite;
  margin: 0 auto 16px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.redirecting-state p {
  color: #64748b;
  margin: 0;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-field label {
  font-size: 14px;
  font-weight: 500;
  color: #334e68;
}

.field-hint {
  color: #627d98;
  font-size: 12px;
}

.field-error {
  color: #dc2626;
  font-size: 12px;
}

.email-display {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: #f8fafc;
  border-radius: 8px;
}

.email-info {
  display: flex;
  align-items: center;
  gap: 12px;
}

.email-avatar {
  width: 40px;
  height: 40px;
  background: linear-gradient(135deg, var(--login-accent, #0967d2) 0%, #47a3f3 100%);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-weight: 600;
}

.email-text {
  color: #475569;
  font-size: 14px;
}

.change-email-btn {
  background: none;
  border: none;
  color: var(--login-accent, #0967d2);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
}

.change-email-btn:hover {
  color: #0552b5;
}

.form-options {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.remember-me {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.remember-me span {
  font-size: 14px;
  color: #475569;
}

.forgot-password {
  font-size: 14px;
  color: var(--login-accent, #0967d2);
  text-decoration: none;
}

.forgot-password:hover {
  color: #0552b5;
}

.login-footer {
  text-align: center;
  color: #627d98;
  font-size: 14px;
  margin: 24px 0 0;
}

/* Override PrimeVue Password component width */
:deep(.p-password) {
  width: 100%;
}

:deep(.p-password-input) {
  width: 100%;
}

/* Override PrimeVue Button to use theme accent color */
:deep(.p-button) {
  background: var(--login-accent, #0967d2);
  border-color: var(--login-accent, #0967d2);
}

:deep(.p-button:not(:disabled):hover) {
  background: color-mix(in srgb, var(--login-accent, #0967d2) 85%, black);
  border-color: color-mix(in srgb, var(--login-accent, #0967d2) 85%, black);
}

:deep(.p-button:not(:disabled):active) {
  background: color-mix(in srgb, var(--login-accent, #0967d2) 75%, black);
  border-color: color-mix(in srgb, var(--login-accent, #0967d2) 75%, black);
}

:deep(.p-button:focus-visible) {
  outline-color: var(--login-accent, #0967d2);
  box-shadow:
    0 0 0 2px #ffffff,
    0 0 0 4px color-mix(in srgb, var(--login-accent, #0967d2) 50%, transparent);
}

:deep(.p-button:disabled) {
  background: color-mix(in srgb, var(--login-accent, #0967d2) 50%, #e2e8f0);
  border-color: color-mix(in srgb, var(--login-accent, #0967d2) 50%, #e2e8f0);
}
</style>
