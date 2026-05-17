import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MCPGatewayClient, MCPGatewayError } from './index'

const mockFetch = vi.fn()

describe('MCPGatewayClient', () => {
  let client: MCPGatewayClient

  beforeEach(() => {
    vi.clearAllMocks()
    // @ts-ignore — inject mock fetch
    client = new MCPGatewayClient({ url: 'http://localhost:8080', apiKey: 'test-key' })
    ;(client as any).fetch = mockFetch
  })

  describe('listTools', () => {
    it('returns aggregated tools', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          jsonrpc: '2.0',
          id: 1,
          result: {
            tools: [
              { name: 'read_file', description: 'Read a file', inputSchema: {}, serverName: 'fs' },
              { name: 'gh_create_issue', description: 'Create issue', inputSchema: {}, serverName: 'github' },
            ],
          },
        }),
      })

      const { tools } = await client.listTools()
      expect(tools).toHaveLength(2)
      expect(tools[0]?.name).toBe('read_file')
      expect(tools[1]?.serverName).toBe('github')
    })

    it('sets X-Gateway-Key header', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ jsonrpc: '2.0', id: 1, result: { tools: [] } }),
      })

      await client.listTools()

      const [_url, opts] = mockFetch.mock.calls[0]
      expect((opts as RequestInit).headers).toMatchObject({ 'X-Gateway-Key': 'test-key' })
    })
  })

  describe('callTool', () => {
    it('sends correct JSON-RPC payload', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          jsonrpc: '2.0',
          id: 1,
          result: { content: [{ type: 'text', text: 'done' }] },
        }),
      })

      const result = await client.callTool({ name: 'read_file', arguments: { path: '/tmp/test' } })
      expect(result.content[0]?.text).toBe('done')

      const body = JSON.parse((mockFetch.mock.calls[0]?.[1] as RequestInit).body as string)
      expect(body.method).toBe('tools/call')
      expect(body.params.name).toBe('read_file')
      expect(body.params.arguments.path).toBe('/tmp/test')
    })

    it('throws MCPGatewayError on JSON-RPC error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          jsonrpc: '2.0',
          id: 1,
          error: { code: -32601, message: 'tool not found: ghost_tool' },
        }),
      })

      await expect(client.callTool({ name: 'ghost_tool' })).rejects.toThrow(MCPGatewayError)
    })
  })

  describe('health', () => {
    it('returns healthy status', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          status: 'healthy',
          servers: [{ name: 'fs', healthy: true, circuitOpen: false, failureCount: 0, lastCheck: '' }],
        }),
      })

      const h = await client.health()
      expect(h.status).toBe('healthy')
      expect(h.servers).toHaveLength(1)
    })

    it('throws on non-OK response', async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 503 })
      await expect(client.health()).rejects.toThrow(MCPGatewayError)
    })
  })

  describe('request ID increments', () => {
    it('uses unique IDs per request', async () => {
      const ids: number[] = []
      mockFetch.mockImplementation(async (_url: string, opts: RequestInit) => {
        const body = JSON.parse(opts.body as string)
        ids.push(body.id)
        return {
          ok: true,
          json: async () => ({ jsonrpc: '2.0', id: body.id, result: { tools: [] } }),
        }
      })

      await client.listTools()
      await client.listTools()
      await client.listTools()

      expect(ids).toEqual([1, 2, 3])
    })
  })
})
