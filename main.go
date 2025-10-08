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
	case "prune":
		if len(os.Args) < 3 {
			printPruneUsage()
			os.Exit(1)
		}
		subcommand := os.Args[2]
		switch subcommand {
		case "containers":
			cmd.PruneContainers(os.Args[3:])
		case "images":
			cmd.PruneImages(os.Args[3:])
		default:
			log.Fatalf("Unknown prune subcommand: %s", subcommand)
		}
	default:
		log.Fatalf("Unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  yourpm switch [config-file]")
	fmt.Println("  yourpm prune <containers|images> [--all]")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  yourpm switch config.example.toml")
	fmt.Println("  yourpm switch  # Uses ~/.yourpm/config.toml by default")
	fmt.Println("  yourpm prune containers  # Clean up unused containers")
	fmt.Println("  yourpm prune containers --all  # Remove all containers (aggressive)")
	fmt.Println("  yourpm prune images  # Clean up dangling images")
	fmt.Println("  yourpm prune images --all  # Remove all unused images (aggressive)")
}

func printPruneUsage() {
	fmt.Println("Usage:")
	fmt.Println("  yourpm prune containers [--all]")
	fmt.Println("  yourpm prune images [--all]")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  yourpm prune containers  # Remove containers not in current config")
	fmt.Println("  yourpm prune containers --all  # Remove all yourpm containers")
	fmt.Println("  yourpm prune images  # Remove dangling Docker images")
	fmt.Println("  yourpm prune images --all  # Remove all unused Docker images")
}
