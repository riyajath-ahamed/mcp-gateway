package main

import (
	"fmt"
	"os"

	"github.com/configkits/mcp-gateway/internal/auth"
)

func main() {
	key, err := auth.GenerateAPIKey(32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(key)
}
