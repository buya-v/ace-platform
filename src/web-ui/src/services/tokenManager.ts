export interface TokenManager {
  setToken(token: string, expiresInSeconds: number): void;
  getToken(): string | null;
  clear(): void;
  onRefreshNeeded(callback: () => Promise<void>): void;
}

export function createTokenManager(): TokenManager {
  let accessToken: string | null = null;
  let expiresAt: number = 0;
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;
  let refreshCallback: (() => Promise<void>) | null = null;

  function scheduleRefresh(delaySeconds: number) {
    if (refreshTimer) clearTimeout(refreshTimer);
    refreshTimer = setTimeout(async () => {
      if (refreshCallback) {
        try {
          await refreshCallback();
        } catch {
          // Refresh failed — session will expire naturally
        }
      }
    }, delaySeconds * 1000);
  }

  return {
    setToken(token: string, expiresInSeconds: number) {
      accessToken = token;
      expiresAt = Date.now() + expiresInSeconds * 1000;
      scheduleRefresh(expiresInSeconds * 0.8);
    },

    getToken(): string | null {
      if (!accessToken || Date.now() >= expiresAt) return null;
      return accessToken;
    },

    clear() {
      accessToken = null;
      expiresAt = 0;
      if (refreshTimer) {
        clearTimeout(refreshTimer);
        refreshTimer = null;
      }
    },

    onRefreshNeeded(callback: () => Promise<void>) {
      refreshCallback = callback;
    },
  };
}

export const tokenManager = createTokenManager();
