package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DockerClient provides an interface for Docker operations
type DockerClient interface {
	// Container lifecycle
	IsRunning(containerName string) bool
	Exists(containerName string) bool
	Start(containerName string) error
	Stop(containerName string) error
	Remove(containerName string) error
	
	// Container creation and execution
	CreateContainer(containerName, image string, opts CreateOptions) error
	ExecCommand(containerName string, opts ExecOptions) error
	
	// Image operations
	ImageExists(image string) bool
	PullImage(image string) error
	GetContainerImage(containerName string) (string, error)
	
	// Container listing
	ListContainers(filters map[string]string) ([]Container, error)
	ListRunningContainers(filters map[string]string) ([]Container, error)
	
	// Image pruning
	PruneImages(aggressive bool) error
}

// CreateOptions holds options for creating containers
type CreateOptions struct {
	Volumes   []string
	WorkDir   string
	Entrypoint string
	Command   []string
}

// ExecOptions holds options for executing commands in containers
type ExecOptions struct {
	Interactive bool
	TTY         bool
	WorkDir     string
	Command     []string
}

// Container represents a Docker container
type Container struct {
	Name   string
	Status string
	Image  string
}

// DefaultDockerClient implements DockerClient using the docker command
type DefaultDockerClient struct{}

// NewDefaultDockerClient creates a new default Docker client
func NewDefaultDockerClient() *DefaultDockerClient {
	return &DefaultDockerClient{}
}

// IsRunning checks if a container is currently running
func (c *DefaultDockerClient) IsRunning(containerName string) bool {
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), containerName)
}

// Exists checks if a container exists (running or stopped)
func (c *DefaultDockerClient) Exists(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), containerName)
}

// Start starts an existing stopped container
func (c *DefaultDockerClient) Start(containerName string) error {
	cmd := exec.Command("docker", "start", containerName)
	return cmd.Run()
}

// Stop stops a running container
func (c *DefaultDockerClient) Stop(containerName string) error {
	cmd := exec.Command("docker", "stop", containerName)
	return cmd.Run()
}

// Remove removes a container (forcefully if running)
func (c *DefaultDockerClient) Remove(containerName string) error {
	cmd := exec.Command("docker", "rm", "-f", containerName)
	return cmd.Run()
}

// CreateContainer creates a new container with specified options
func (c *DefaultDockerClient) CreateContainer(containerName, image string, opts CreateOptions) error {
	args := []string{"run", "-d", "--name", containerName}
	
	if opts.Entrypoint != "" {
		args = append(args, "--entrypoint", opts.Entrypoint)
	}
	
	// Add volume mounts
	for _, volume := range opts.Volumes {
		args = append(args, "-v", volume)
	}
	
	// Add working directory
	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}
	
	// Add image
	args = append(args, image)
	
	// Add command (default to keep-alive)
	if len(opts.Command) > 0 {
		args = append(args, opts.Command...)
	} else {
		args = append(args, "tail", "-f", "/dev/null")
	}
	
	cmd := exec.Command("docker", args...)
	return cmd.Run()
}

// ExecCommand executes a command in a running container
func (c *DefaultDockerClient) ExecCommand(containerName string, opts ExecOptions) error {
	args := []string{"exec"}
	
	if opts.Interactive && opts.TTY {
		args = append(args, "-it")
	} else if opts.Interactive {
		args = append(args, "-i")
	}
	
	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}
	
	args = append(args, containerName)
	args = append(args, opts.Command...)
	
	cmd := exec.Command("docker", args...)
	return cmd.Run()
}

// ImageExists checks if an image exists locally
func (c *DefaultDockerClient) ImageExists(image string) bool {
	cmd := exec.Command("docker", "image", "inspect", image)
	err := cmd.Run()
	return err == nil
}

// PullImage pulls an image from registry
func (c *DefaultDockerClient) PullImage(image string) error {
	cmd := exec.Command("docker", "pull", image)
	// Show docker pull output to user
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GetContainerImage returns the image used by a container
func (c *DefaultDockerClient) GetContainerImage(containerName string) (string, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.Config.Image}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// ListContainers lists all containers matching the filters
func (c *DefaultDockerClient) ListContainers(filters map[string]string) ([]Container, error) {
	args := []string{"ps", "-a", "--format", "{{.Names}}|{{.Status}}|{{.Image}}"}
	
	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}
	
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	return parseContainerList(string(output)), nil
}

// ListRunningContainers lists only running containers matching the filters
func (c *DefaultDockerClient) ListRunningContainers(filters map[string]string) ([]Container, error) {
	args := []string{"ps", "--format", "{{.Names}}|{{.Status}}|{{.Image}}"}
	
	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}
	
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	return parseContainerList(string(output)), nil
}

// PruneImages removes unused images
func (c *DefaultDockerClient) PruneImages(aggressive bool) error {
	args := []string{"image", "prune", "-f"}
	if aggressive {
		args = append(args, "-a")
	}
	
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// parseContainerList parses docker ps output into Container structs
func parseContainerList(output string) []Container {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []Container{}
	}
	
	var containers []Container
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "|")
		if len(parts) >= 3 {
			containers = append(containers, Container{
				Name:   parts[0],
				Status: parts[1],
				Image:  parts[2],
			})
		}
	}
	
	return containers
}