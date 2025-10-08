package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crbroughton/pkg-exploration/pkg/config"
	"github.com/crbroughton/pkg-exploration/pkg/containers"
)

func main() {
	// Get the command name from how this binary was called
	commandName := filepath.Base(os.Args[0])
	
	// Load config and manifest
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".yourpm")
	
	cfg, err := config.LoadConfig(filepath.Join(baseDir, "config.toml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	
	manifest, err := containers.LoadContainerManifest(filepath.Join(baseDir, "containers.toml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading container manifest: %v\n", err)
		os.Exit(1)
	}

	// Find which container this command belongs to
	containerName, containerConfig, containerDef := findContainerForCommand(commandName, cfg, manifest)
	if containerName == "" {
		fmt.Fprintf(os.Stderr, "Command '%s' not found in any container\n", commandName)
		os.Exit(1)
	}

	// Build image name from config
	image := fmt.Sprintf("%s:%s", containerConfig.Image, containerConfig.Version)
	
	// Ensure container is running
	containerFullName := fmt.Sprintf("yourpm-%s", containerName)
	if err := ensureContainerRunning(containerFullName, image, containerDef); err != nil {
		fmt.Fprintf(os.Stderr, "Error ensuring container is running: %v\n", err)
		os.Exit(1)
	}

	// Execute command directly
	dockerArgs := buildDockerExecArgs(containerFullName, containerDef, commandName, os.Args[1:])
	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		os.Exit(1)
	}
}

func findContainerForCommand(commandName string, cfg *config.Config, manifest *containers.ContainerManifest) (string, config.ContainerConfig, *containers.ContainerDefinition) {
	for containerName, containerConfig := range cfg.Containers {
		if containerDef, exists := manifest.Containers[containerName]; exists {
			for _, cmd := range containerDef.Commands {
				if cmd == commandName {
					return containerName, containerConfig, &containerDef
				}
			}
		}
	}
	return "", config.ContainerConfig{}, nil
}

func buildDockerExecArgs(containerName string, containerDef *containers.ContainerDefinition, command string, args []string) []string {
	dockerArgs := []string{"exec"}
	
	// Add interactive flag (avoid TTY issues in automation)
	dockerArgs = append(dockerArgs, "-i")
	
	// Add working directory if specified
	if containerDef.WorkDir != "" {
		dockerArgs = append(dockerArgs, "-w", containerDef.WorkDir)
	}
	
	// Add container name and command
	dockerArgs = append(dockerArgs, containerName, command)
	dockerArgs = append(dockerArgs, args...)
	
	return dockerArgs
}

func ensureContainerRunning(containerName, image string, containerDef *containers.ContainerDefinition) error {
	// Check if container exists
	if containerExists(containerName) {
		// Check if existing container uses correct image
		currentImage, err := getContainerImage(containerName)
		if err != nil {
			return fmt.Errorf("failed to get container image: %w", err)
		}
		
		// If image changed, remove old container and create new one
		if currentImage != image {
			fmt.Printf("  📦 Image changed from %s to %s, recreating container...\n", currentImage, image)
			if err := removeContainer(containerName); err != nil {
				return fmt.Errorf("failed to remove old container: %w", err)
			}
			return createContainer(containerName, image, containerDef)
		}
		
		// Container exists with correct image, ensure it's running
		if !isContainerRunning(containerName) {
			return startContainer(containerName)
		}
		return nil
	}
	
	// Create new container
	return createContainer(containerName, image, containerDef)
}

func isContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), containerName)
}

func containerExists(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), containerName)
}

func startContainer(containerName string) error {
	cmd := exec.Command("docker", "start", containerName)
	return cmd.Run()
}

func getContainerImage(containerName string) (string, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.Config.Image}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func removeContainer(containerName string) error {
	cmd := exec.Command("docker", "rm", "-f", containerName)
	return cmd.Run()
}

func createContainer(containerName, image string, containerDef *containers.ContainerDefinition) error {
	args := []string{"run", "-d", "--name", containerName, "--entrypoint", ""}
	
	// Add volume mounts
	for _, volume := range containerDef.Volumes {
		args = append(args, "-v", volume)
	}
	
	// Add working directory
	if containerDef.WorkDir != "" {
		args = append(args, "-w", containerDef.WorkDir)
	}
	
	// Add image and keep-alive command
	args = append(args, image, "tail", "-f", "/dev/null")
	
	cmd := exec.Command("docker", args...)
	return cmd.Run()
}

func isTerminal() bool {
	if fileInfo, err := os.Stdin.Stat(); err == nil {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return false
}