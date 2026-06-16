package version

// String is set at build time via ldflags:
//
//	-X github.com/configkits/mcp-gateway/internal/version.String=x.y.z
var String = "dev"
