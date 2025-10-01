package main

import (
	"fmt"
	"log"
	"os"

	"github.com/crbroughton/pkg-exploration/pkg/cmd"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "switch":
		cmd.Switch(os.Args[2:])
	default:
		log.Fatalf("Unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  yourpm switch [config-file]")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  yourpm switch ~/.yourpm/config.toml")
	fmt.Println("  yourpm switch  # Uses ~/.yourpm/config.toml by default")
}
