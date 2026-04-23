declare global {
  interface Window {
    __GARUDAX_CONFIG__?: Partial<AppConfig>;
  }
}

export interface AppConfig {
  API_BASE_URL: string;
  WS_BASE_URL: string;
  HEALTH_POLL_INTERVAL: number;
  AUTH_TOKEN_REFRESH_BUFFER: number;
}

function getDefaultWsUrl(): string {
  try {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/api/v1/ws`;
  } catch {
    return 'ws://localhost:8080/api/v1/ws';
  }
}

export function getConfig(): AppConfig {
  const defaults: AppConfig = {
    API_BASE_URL: '/api/v1',
    WS_BASE_URL: getDefaultWsUrl(),
    HEALTH_POLL_INTERVAL: 15000,
    AUTH_TOKEN_REFRESH_BUFFER: 60,
  };

  const runtime = window.__GARUDAX_CONFIG__ ?? {};
  return { ...defaults, ...runtime };
}
