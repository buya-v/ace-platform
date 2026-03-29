import { tokenManager } from './tokenManager';

const API_BASE_URL = '/api/v1';

export class AuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'AuthError';
  }
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function attemptRefresh(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE_URL}/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    });
    if (!res.ok) return false;
    const data = await res.json();
    tokenManager.setToken(data.access_token || data.AccessToken, data.expires_in || data.ExpiresIn);
    return true;
  } catch {
    return false;
  }
}

export async function apiRequest<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = tokenManager.getToken();
  if (!token) throw new AuthError('No valid token');

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
      ...options.headers,
    },
  });

  if (response.status === 401) {
    const refreshed = await attemptRefresh();
    if (refreshed) return apiRequest<T>(path, options);
    throw new AuthError('Session expired');
  }

  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ApiError(response.status, body.error || 'Unknown error', body.code);
  }

  return response.json();
}

export async function login(email: string, password: string): Promise<{ user: { id: string; email: string; displayName: string; roles: string[]; participantId: string } }> {
  const response = await fetch(`${API_BASE_URL}/auth/login`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });

  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ApiError(response.status, body.error || 'Login failed', body.code);
  }

  const data = await response.json();
  tokenManager.setToken(data.access_token || data.AccessToken, data.expires_in || data.ExpiresIn);
  return { user: data.user };
}

export async function logout(): Promise<void> {
  try {
    await fetch(`${API_BASE_URL}/auth/logout`, {
      method: 'POST',
      credentials: 'include',
    });
  } finally {
    tokenManager.clear();
  }
}

export async function silentRefresh(): Promise<{ user: { id: string; email: string; displayName: string; roles: string[]; participantId: string } } | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
    });
    if (!res.ok) return null;
    const data = await res.json();
    tokenManager.setToken(data.access_token || data.AccessToken, data.expires_in || data.ExpiresIn);
    return { user: data.user };
  } catch {
    return null;
  }
}
