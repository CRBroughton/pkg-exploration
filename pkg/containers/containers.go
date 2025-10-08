package containers

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/BurntSushi/toml"
)

type ContainerManifest struct {
	Containers map[string]ContainerDefinition `toml:"containers"`
}

type ContainerDefinition struct {
	Description string   `toml:"description"`
	Commands    []string `toml:"commands"`
	WorkDir     string   `toml:"workdir"`
	Volumes     []string `toml:"volumes"`
}

func LoadContainerManifest(path string) (*ContainerManifest, error) {
	var m ContainerManifest
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return nil, fmt.Errorf("failed to parse container manifest: %w", err)
	}
	return &m, nil
}

func (m *ContainerManifest) GetContainer(name string) (*ContainerDefinition, error) {
	container, ok := m.Containers[name]
	if !ok {
		return nil, fmt.Errorf("container %s not found in manifest", name)
	}
	return &container, nil
}

func (c *ContainerDefinition) CreateDockerCommand(containerName, image, command string, args []string) []string {
	dockerArgs := []string{"run", "--rm", "-i"}
	
	// Add volume mounts
	for _, volume := range c.Volumes {
		dockerArgs = append(dockerArgs, "-v", volume)
	}
	
	// Set working directory if specified
	if c.WorkDir != "" {
		dockerArgs = append(dockerArgs, "-w", c.WorkDir)
	}
	
	// Add the image
	dockerArgs = append(dockerArgs, image)
	
	// Add the command and arguments
	dockerArgs = append(dockerArgs, command)
	dockerArgs = append(dockerArgs, args...)
	
	return dockerArgs
}

func (c *ContainerDefinition) GenerateGoWrapper(containerName, image, command string) string {
	return fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"os/exec"
	"github.com/crbroughton/pkg-exploration/pkg/containers"
)

func main() {
	// Check if Docker is available
	if !isDockerAvailable() {
		fmt.Fprintf(os.Stderr, "Error: Docker is required but not installed or not in PATH\n")
		os.Exit(1)
	}

	// Container definition (embedded from manifest)
	containerDef := &containers.ContainerDefinition{
		Commands: %s,
		WorkDir:  "%s",
		Volumes:  %s,
	}

	// Execute the command
	if err := containerDef.ExecuteCommand("%s", "%s", "%s", os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %%v\n", err)
		os.Exit(1)
	}
}

func isDockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}
`, 
		formatStringSlice(c.Commands),
		c.WorkDir,
		formatStringSlice(c.Volumes),
		containerName,
		image,
		command)
}

func formatStringSlice(slice []string) string {
	if len(slice) == 0 {
		return "[]string{}"
	}
	
	var result strings.Builder
	result.WriteString("[]string{")
	for i, s := range slice {
		if i > 0 {
			result.WriteString(", ")
		}
		result.WriteString(fmt.Sprintf(`"%s"`, s))
	}
	result.WriteString("}")
	return result.String()
}

// Legacy method for backwards compatibility
func (c *ContainerDefinition) GenerateWrapperScript(containerName, image, command string) string {
	return fmt.Sprintf(`#!/bin/bash
# Generated wrapper for %s in container %s

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is required but not installed or not in PATH"
    exit 1
fi

CONTAINER_NAME="yourpm-%s"

# Check if container exists and is running
if ! docker ps --format "table {{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
    # Container not running, check if it exists but is stopped
    if docker ps -a --format "table {{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
        # Container exists but stopped, start it
        docker start "${CONTAINER_NAME}" >/dev/null 2>&1
    else
        # Container doesn't exist, create and start it
        docker run -d \
            --name "${CONTAINER_NAME}" \
            --entrypoint="" \
            %s \
            %s \
            "%s" \
            tail -f /dev/null >/dev/null 2>&1
    fi
fi

# Execute command in the running container
if [ -t 0 ]; then
    exec docker exec -it %s "${CONTAINER_NAME}" %s "$@"
else
    exec docker exec -i %s "${CONTAINER_NAME}" %s "$@"
fi
`,
		command, containerName, containerName,
		c.formatVolumeMounts(),
		c.formatWorkDir(), 
		image,
		c.formatWorkDirExec(),
		command,
		c.formatWorkDirExec(),
		command)
}

func (c *ContainerDefinition) formatVolumeMounts() string {
	var mounts []string
	for _, volume := range c.Volumes {
		mounts = append(mounts, fmt.Sprintf(`"-v" "%s"`, volume))
	}
	return strings.Join(mounts, " ")
}

func (c *ContainerDefinition) formatWorkDir() string {
	if c.WorkDir == "" {
		return ""
	}
	return fmt.Sprintf(`"-w" "%s"`, c.WorkDir)
}

func (c *ContainerDefinition) formatWorkDirExec() string {
	if c.WorkDir == "" {
		return ""
	}
	return fmt.Sprintf(`--workdir="%s"`, c.WorkDir)
}

// ExecuteCommand runs a command in the container using Go's exec package
func (c *ContainerDefinition) ExecuteCommand(containerName, image, command string, args []string) error {
	containerFullName := fmt.Sprintf("yourpm-%s", containerName)

	// Ensure container is running
	if err := c.ensureContainerRunning(containerFullName, image); err != nil {
		return fmt.Errorf("failed to ensure container is running: %w", err)
	}

	// Build docker exec command
	dockerArgs := []string{"exec"}
	
	// Add TTY if stdin is a terminal
	if isTerminal() {
		dockerArgs = append(dockerArgs, "-it")
	} else {
		dockerArgs = append(dockerArgs, "-i")
	}

	// Add working directory
	if c.WorkDir != "" {
		dockerArgs = append(dockerArgs, "--workdir", c.WorkDir)
	}

	// Add container name and command
	dockerArgs = append(dockerArgs, containerFullName, command)
	dockerArgs = append(dockerArgs, args...)

	// Execute with proper signal handling
	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// ensureContainerRunning makes sure the container exists and is running
func (c *ContainerDefinition) ensureContainerRunning(containerName, image string) error {
	// Check if container is running
	if c.isContainerRunning(containerName) {
		return nil
	}

	// Check if container exists but is stopped
	if c.containerExists(containerName) {
		return c.startContainer(containerName)
	}

	// Create new container
	return c.createContainer(containerName, image)
}

// isContainerRunning checks if container is currently running
func (c *ContainerDefinition) isContainerRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), containerName)
}

// containerExists checks if container exists (running or stopped)
func (c *ContainerDefinition) containerExists(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), containerName)
}

// startContainer starts an existing stopped container
func (c *ContainerDefinition) startContainer(containerName string) error {
	cmd := exec.Command("docker", "start", containerName)
	_, err := cmd.Output()
	return err
}

// createContainer creates and starts a new container
func (c *ContainerDefinition) createContainer(containerName, image string) error {
	args := []string{"run", "-d", "--name", containerName, "--entrypoint", ""}
	
	// Add volume mounts
	for _, volume := range c.Volumes {
		args = append(args, "-v", volume)
	}
	
	// Add working directory
	if c.WorkDir != "" {
		args = append(args, "-w", c.WorkDir)
	}
	
	// Add image and command
	args = append(args, image, "tail", "-f", "/dev/null")
	
	cmd := exec.Command("docker", args...)
	_, err := cmd.Output()
	return err
}

// isTerminal checks if stdin is a terminal
func isTerminal() bool {
	if fileInfo, err := os.Stdin.Stat(); err == nil {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return false
}