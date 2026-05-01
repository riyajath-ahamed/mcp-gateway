package registry

// InjectTools is a test helper that directly sets the tool list for a named server.
// It bypasses network discovery so unit tests don't require live backends.
func InjectTools(r *Registry, serverName string, tools []MCPTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.servers {
		if s.Config.Name == serverName {
			s.mu.Lock()
			s.Tools = tools
			s.Healthy = true
			s.CircuitOpen = false
			s.FailureCount = 0
			s.mu.Unlock()
			return
		}
	}
}
