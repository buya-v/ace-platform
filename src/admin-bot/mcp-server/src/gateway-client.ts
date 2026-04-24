/**
 * GatewayClient — HTTP client for the GarudaX gateway REST API.
 *
 * Handles JWT authentication (auto-login on first request) and provides
 * a generic request method that all MCP tools delegate to.
 */

export interface GatewayClientOptions {
  baseUrl: string;
  email?: string;
  password?: string;
}

export class GatewayClient {
  private baseUrl: string;
  private email?: string;
  private password?: string;
  private token?: string;

  constructor(opts: GatewayClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/+$/, "");
    this.email = opts.email;
    this.password = opts.password;
  }

  /**
   * Authenticate against the gateway and store the JWT token.
   */
  async login(email: string, password: string): Promise<void> {
    const url = `${this.baseUrl}/api/v1/auth/login`;
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });

    if (!res.ok) {
      const body = await res.text();
      throw new Error(
        `Login failed (${res.status}): ${body}`,
      );
    }

    const data = (await res.json()) as Record<string, unknown>;
    // Accept both snake_case and PascalCase field names
    const token = (data.access_token ?? data.AccessToken ?? data.token) as
      | string
      | undefined;

    if (!token) {
      throw new Error(
        `Login response did not contain a token: ${JSON.stringify(data)}`,
      );
    }

    this.token = token;
  }

  /**
   * Send an authenticated request to the gateway.
   * Auto-logs-in on the first call if credentials were provided via env.
   * Optional `extraHeaders` are merged into the request headers after
   * Authorization is set, allowing callers to inject tenant context etc.
   */
  async request(
    method: string,
    path: string,
    body?: unknown,
    extraHeaders?: Record<string, string>,
  ): Promise<unknown> {
    // Auto-login once if we have credentials but no token yet
    if (!this.token && this.email && this.password) {
      await this.login(this.email, this.password);
    }

    const url = `${this.baseUrl}${path}`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }
    if (extraHeaders) {
      Object.assign(headers, extraHeaders);
    }

    const init: RequestInit = { method, headers };
    if (body !== undefined) {
      init.body = JSON.stringify(body);
    }

    const res = await fetch(url, init);

    // Try to parse JSON; fall back to text
    const contentType = res.headers.get("content-type") ?? "";
    let responseBody: unknown;
    if (contentType.includes("application/json")) {
      responseBody = await res.json();
    } else {
      responseBody = await res.text();
    }

    if (!res.ok) {
      const preview =
        typeof responseBody === "string"
          ? responseBody.slice(0, 500)
          : JSON.stringify(responseBody, null, 2).slice(0, 500);
      throw new Error(
        `Gateway ${method} ${path} failed (${res.status}): ${preview}`,
      );
    }

    return responseBody;
  }
}
