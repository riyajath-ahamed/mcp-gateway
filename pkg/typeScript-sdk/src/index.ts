/**
 * @configkits/mcp-gateway-client
 *
 * TypeScript client SDK for mcp-gateway.
 * One client, all your MCP servers.
 */

export interface MCPGatewayClientOptions {
  /** Gateway base URL, e.g. "https://gateway.yourapp.com" */
  url: string;
  /** API key sent as X-Gateway-Key header */
  apiKey?: string;
  /** Custom headers merged into every request */
  headers?: Record<string, string>;
  /** Request timeout in milliseconds (default: 60000) */
  timeoutMs?: number;
}

export interface MCPTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  /** Which backend server owns this tool */
  serverName: string;
}

export interface ToolCallResult {
  content: Array<{
    type: "text" | "image" | "resource";
    text?: string;
    data?: string;
    mimeType?: string;
  }>;
  isError?: boolean;
}

export interface ListToolsResult {
  tools: MCPTool[];
}

export interface CallToolParams {
  name: string;
  arguments?: Record<string, unknown>;
}

interface JsonRPCResponse<T> {
  jsonrpc: "2.0";
  id: unknown;
  result?: T;
  error?: { code: number; message: string };
}

export class MCPGatewayClient {
  private readonly url: string;
  private readonly defaultHeaders: Record<string, string>;
  private readonly timeoutMs: number;
  private requestId = 0;

  constructor(options: MCPGatewayClientOptions) {
    this.url = options.url.replace(/\/$/, "");
    this.timeoutMs = options.timeoutMs ?? 60_000;
    this.defaultHeaders = {
      "Content-Type": "application/json",
      ...(options.apiKey ? { "X-Gateway-Key": options.apiKey } : {}),
      ...(options.headers ?? {}),
    };
  }

  /**
   * List all tools aggregated from all backend MCP servers.
   */
  async listTools(): Promise<ListToolsResult> {
    const res = await this.rpc<ListToolsResult>("tools/list", {});
    return res;
  }

  /**
   * Call a tool by name. The gateway routes the call to the correct backend automatically.
   */
  async callTool(params: CallToolParams): Promise<ToolCallResult> {
    return this.rpc<ToolCallResult>("tools/call", {
      name: params.name,
      arguments: params.arguments ?? {},
    });
  }

  /**
   * Stream a tool call response. Yields chunks as they arrive.
   * Requires the gateway and backend to support SSE streaming.
   */
  async *callToolStream(
    params: CallToolParams
  ): AsyncGenerator<string, void, unknown> {
    const body = JSON.stringify({
      jsonrpc: "2.0",
      id: ++this.requestId,
      method: "tools/call",
      params: {
        name: params.name,
        arguments: params.arguments ?? {},
      },
    });

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const response = await fetch(`${this.url}/mcp`, {
        method: "POST",
        headers: {
          ...this.defaultHeaders,
          Accept: "text/event-stream",
        },
        body,
        signal: controller.signal,
      });

      if (!response.ok || !response.body) {
        throw new MCPGatewayError(
          `HTTP ${response.status}`,
          response.status,
          -32002
        );
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        const chunk = decoder.decode(value, { stream: true });
        // Parse SSE data lines
        for (const line of chunk.split("\n")) {
          if (line.startsWith("data: ")) {
            yield line.slice(6);
          }
        }
      }
    } finally {
      clearTimeout(timer);
    }
  }

  /**
   * Check gateway and backend health.
   */
  async health(): Promise<{
    status: "healthy" | "degraded";
    servers: Array<{
      name: string;
      healthy: boolean;
      circuitOpen: boolean;
      failureCount: number;
      lastCheck: string;
    }>;
  }> {
    const response = await this.fetch(`${this.url}/health`, {
      method: "GET",
      headers: this.defaultHeaders,
    });
    if (!response.ok) {
      throw new MCPGatewayError(`Health check failed: HTTP ${response.status}`, response.status, -32000);
    }
    return response.json();
  }

  private async rpc<T>(method: string, params: unknown): Promise<T> {
    const body = JSON.stringify({
      jsonrpc: "2.0",
      id: ++this.requestId,
      method,
      params,
    });

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const response = await this.fetch(`${this.url}/mcp`, {
        method: "POST",
        headers: this.defaultHeaders,
        body,
        signal: controller.signal,
      });

      const data: JsonRPCResponse<T> = await response.json();

      if (data.error) {
        throw new MCPGatewayError(data.error.message, response.status, data.error.code);
      }

      if (data.result === undefined) {
        throw new MCPGatewayError("Empty result from gateway", response.status, -32000);
      }

      return data.result;
    } catch (err) {
      if (err instanceof MCPGatewayError) throw err;
      if ((err as Error).name === "AbortError") {
        throw new MCPGatewayError(`Request timeout after ${this.timeoutMs}ms`, 0, -32000);
      }
      throw new MCPGatewayError(`Network error: ${(err as Error).message}`, 0, -32000);
    } finally {
      clearTimeout(timer);
    }
  }

  private fetch = globalThis.fetch.bind(globalThis);
}

export class MCPGatewayError extends Error {
  constructor(
    message: string,
    public readonly httpStatus: number,
    public readonly rpcCode: number
  ) {
    super(message);
    this.name = "MCPGatewayError";
  }
}

// Convenience factory
export function createGatewayClient(options: MCPGatewayClientOptions): MCPGatewayClient {
  return new MCPGatewayClient(options);
}
