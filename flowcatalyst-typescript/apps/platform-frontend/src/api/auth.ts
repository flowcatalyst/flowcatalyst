import { useAuthStore, type User } from '@/stores/auth';
import router from '@/router';

// Auth endpoints are at /auth/* (not /api/auth/*)
const AUTH_URL = '/auth';

interface LoginCredentials {
  email: string;
  password: string;
}

interface LoginResponse {
  principalId: string;
  name: string;
  email: string;
  roles: string[];
  clientId: string | null;
}

export interface DomainCheckResponse {
  authMethod: 'internal' | 'external';
  loginUrl?: string;
  idpIssuer?: string;
}

function mapLoginResponseToUser(response: LoginResponse): User {
  return {
    id: response.principalId,
    email: response.email,
    name: response.name,
    clientId: response.clientId,
    roles: response.roles,
    permissions: [],  // Permissions are loaded separately or derived from roles
  };
}

export async function checkEmailDomain(email: string): Promise<DomainCheckResponse> {
  const response = await fetch(`${AUTH_URL}/check-domain`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error('Failed to check email domain');
  }

  return response.json();
}

export async function checkSession(): Promise<boolean> {
  const authStore = useAuthStore();
  authStore.setLoading(true);

  try {
    const response = await fetch(`${AUTH_URL}/me`, {
      credentials: 'include',
    });

    if (!response.ok) {
      authStore.clearAuth();
      return false;
    }

    const data: LoginResponse = await response.json();
    authStore.setUser(mapLoginResponseToUser(data));
    return true;
  } catch {
    authStore.clearAuth();
    return false;
  }
}

export async function login(credentials: LoginCredentials): Promise<void> {
  const authStore = useAuthStore();
  authStore.setLoading(true);
  authStore.setError(null);

  try {
    const response = await fetch(`${AUTH_URL}/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(credentials),
      credentials: 'include',
    });

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}));
      throw new Error(errorData.error || 'Login failed. Please check your credentials.');
    }

    const data: LoginResponse = await response.json();
    authStore.setUser(mapLoginResponseToUser(data));

    // Check if this is part of an OAuth flow - redirect back to /oauth/authorize
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.get('oauth') === 'true') {
      // Rebuild OAuth authorize URL with all params
      const oauthParams = new URLSearchParams();
      const oauthFields = ['response_type', 'client_id', 'redirect_uri', 'scope', 'state',
                          'code_challenge', 'code_challenge_method', 'nonce'];
      for (const field of oauthFields) {
        const value = urlParams.get(field);
        if (value) oauthParams.set(field, value);
      }
      window.location.href = `/oauth/authorize?${oauthParams.toString()}`;
      return;
    }

    // Normal login - go to dashboard
    await router.replace('/dashboard');
  } catch (error: any) {
    authStore.setLoading(false);
    authStore.setError(error.message);
    throw error;
  }
}

export async function logout(): Promise<void> {
  const authStore = useAuthStore();

  try {
    await fetch(`${AUTH_URL}/logout`, {
      method: 'POST',
      credentials: 'include',
    });
  } catch {
    // Ignore errors - clear local state anyway
  }

  authStore.clearAuth();
  // Use replace to clear navigation history on logout
  await router.replace('/auth/login');
}

export async function switchClient(clientId: string): Promise<void> {
  const authStore = useAuthStore();

  try {
    const response = await fetch(`${AUTH_URL}/client/${clientId}`, {
      method: 'POST',
      credentials: 'include',
    });

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}));
      throw new Error(errorData.message || 'Failed to switch client');
    }

    authStore.selectClient(clientId);
  } catch (error: any) {
    authStore.setError(error.message);
    throw error;
  }
}
