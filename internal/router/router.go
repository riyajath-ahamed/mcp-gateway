package router

import (
	"fmt"
	"regexp"
	"sync/atomic"

	"github.com/configkits/mcp-gateway/internal/config"
)

// Rule describes how to match a tool name to a backend server.
type Rule struct {
	ServerName string
	Prefix     string
	Pattern    *regexp.Regexp
	Weight     int
}

// Router resolves tool names to backend server names.
type Router struct {
	rules   []Rule
	counter atomic.Uint64 // for round-robin
}

// New compiles routing rules from server configs.
func New(servers []config.ServerConfig) (*Router, error) {
	r := &Router{}
	for _, s := range servers {
		rule := Rule{ServerName: s.Name, Weight: 1}
		if s.Routing != nil {
			rule.Prefix = s.Routing.Prefix
			if s.Routing.Pattern != "" {
				re, err := regexp.Compile(s.Routing.Pattern)
				if err != nil {
					return nil, fmt.Errorf("server %q: invalid routing pattern %q: %w",
						s.Name, s.Routing.Pattern, err)
				}
				rule.Pattern = re
			}
			if s.Routing.Weight > 0 {
				rule.Weight = s.Routing.Weight
			}
		}
		r.rules = append(r.rules, rule)
	}
	return r, nil
}

// MatchResult holds the result of routing a tool name.
type MatchResult struct {
	// ServerName of the matched backend
	ServerName string
	// MatchedBy describes what rule triggered the match
	MatchedBy string
}

// Route finds the best backend server for a given tool name.
// Returns (nil, nil) if no explicit rule matches — caller should fall back to
// iterating the registry by tool ownership.
func (r *Router) Route(toolName string) (*MatchResult, error) {
	// 1. Prefix rules (highest priority)
	for _, rule := range r.rules {
		if rule.Prefix != "" && len(toolName) >= len(rule.Prefix) {
			if toolName[:len(rule.Prefix)] == rule.Prefix {
				return &MatchResult{
					ServerName: rule.ServerName,
					MatchedBy:  "prefix:" + rule.Prefix,
				}, nil
			}
		}
	}

	// 2. Regex pattern rules
	for _, rule := range r.rules {
		if rule.Pattern != nil && rule.Pattern.MatchString(toolName) {
			return &MatchResult{
				ServerName: rule.ServerName,
				MatchedBy:  "pattern:" + rule.Pattern.String(),
			}, nil
		}
	}

	// No explicit rule — return nil so caller uses capability lookup
	return nil, nil
}

// RoundRobin picks a server from the provided slice using weighted round-robin.
// Weights are respected: a server with weight=3 gets 3× the requests of weight=1.
func (r *Router) RoundRobin(serverNames []string, weights []int) string {
	if len(serverNames) == 0 {
		return ""
	}
	if len(serverNames) == 1 {
		return serverNames[0]
	}

	// Build expanded list respecting weights
	var pool []string
	for i, name := range serverNames {
		w := 1
		if i < len(weights) && weights[i] > 0 {
			w = weights[i]
		}
		for j := 0; j < w; j++ {
			pool = append(pool, name)
		}
	}

	idx := r.counter.Add(1) - 1
	return pool[idx%uint64(len(pool))]
}
