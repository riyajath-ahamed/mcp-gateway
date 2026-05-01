/**
 * examples/typescript/basic-usage.ts
 *
 * Demonstrates the @configkits/mcp-gateway-client SDK.
 * Run: npx tsx examples/typescript/basic-usage.ts
 */
import { MCPGatewayClient, MCPGatewayError } from '@configkits/mcp-gateway-client'

const gateway = new MCPGatewayClient({
  url: process.env.MCP_GATEWAY_URL ?? 'http://localhost:8080',
  apiKey: process.env.MCP_GATEWAY_KEY,
})

async function main() {
  console.log('mcp-gateway SDK example\n')

  // 1. Check gateway health
  console.log('1. Health check:')
  const health = await gateway.health()
  console.log(`   status: ${health.status}`)
  health.servers.forEach(s => {
    const icon = s.healthy ? '✓' : '✗'
    console.log(`   ${icon} ${s.name} (failures: ${s.failureCount})`)
  })

  // 2. List all aggregated tools
  console.log('\n2. Aggregated tools:')
  const { tools } = await gateway.listTools()
  console.log(`   Total: ${tools.length} tools from ${new Set(tools.map(t => t.serverName)).size} servers`)

  // Group by server
  const byServer = tools.reduce((acc, t) => {
    acc[t.serverName] = (acc[t.serverName] ?? 0) + 1
    return acc
  }, {} as Record<string, number>)
  Object.entries(byServer).forEach(([server, count]) => {
    console.log(`   ${server}: ${count} tools`)
  })

  // 3. Call a tool (route is automatic)
  if (tools.length > 0) {
    const firstTool = tools[0]!
    console.log(`\n3. Calling tool: ${firstTool.name}`)
    try {
      const result = await gateway.callTool({
        name: firstTool.name,
        arguments: {},
      })
      console.log(`   Result: ${JSON.stringify(result.content[0]).slice(0, 80)}...`)
    } catch (err) {
      if (err instanceof MCPGatewayError) {
        console.log(`   Error (${err.rpcCode}): ${err.message}`)
      }
    }
  }

  // 4. Streaming example
  console.log('\n4. Streaming tool call:')
  try {
    let chunks = 0
    for await (const chunk of gateway.callToolStream({ name: 'fs_read_file', arguments: { path: '/etc/hosts' } })) {
      chunks++
      process.stdout.write('.')
      if (chunks >= 5) break
    }
    console.log(`\n   Received ${chunks} chunks`)
  } catch {
    console.log('   (streaming not available for this backend)')
  }
}

main().catch(err => {
  console.error('Fatal:', err)
  process.exit(1)
})
