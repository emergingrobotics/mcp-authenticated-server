package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/config"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to config file")
	flag.Parse()

	_, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("config is valid")
}
