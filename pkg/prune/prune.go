package prune

import (
	"fmt"

	"github.com/crbroughton/pkg-exploration/pkg/config"
	"github.com/crbroughton/pkg-exploration/pkg/docker"
)

// PruneService handles pruning operations for containers and images
type PruneService struct {
	dockerClient docker.DockerClient
}

// NewPruneService creates a new prune service
func NewPruneService(dockerClient docker.DockerClient) *PruneService {
	return &PruneService{
		dockerClient: dockerClient,
	}
}

// NewDefaultPruneService creates a prune service with default Docker client
func NewDefaultPruneService() *PruneService {
	return NewPruneService(docker.NewDefaultDockerClient())
}

// PruneContainersOptions holds options for container pruning
type PruneContainersOptions struct {
	Aggressive bool
	Config     *config.Config
}

// PruneContainers removes containers based on the provided options
func (p *PruneService) PruneContainers(opts PruneContainersOptions) error {
	// Get all yourpm containers
	filters := map[string]string{"name": "yourpm-"}
	containers, err := p.dockerClient.ListContainers(filters)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Printf("  ✓ No yourpm containers found\n")
		return nil
	}

	configContainers := make(map[string]bool)

	// Build map of containers that are in current config
	if opts.Config != nil {
		for containerName := range opts.Config.Containers {
			configContainers[fmt.Sprintf("yourpm-%s", containerName)] = true
		}
	}

	removedCount := 0
	for _, container := range containers {
		shouldKeep := false
		if !opts.Aggressive && configContainers[container.Name] {
			// Check if container is actually running
			if p.dockerClient.IsRunning(container.Name) {
				shouldKeep = true
				fmt.Printf("  ✓ Keeping running container: %s\n", container.Name)
			} else {
				fmt.Printf("  🔄 Container %s is stopped, will be removed\n", container.Name)
			}
		}

		if shouldKeep {
			continue
		}

		fmt.Printf("  🗑️  Removing container: %s\n", container.Name)
		if err := p.dockerClient.Remove(container.Name); err != nil {
			fmt.Printf("     ⚠️  Failed to remove %s: %v\n", container.Name, err)
		} else {
			removedCount++
		}
	}

	if removedCount > 0 {
		fmt.Printf("  ✓ Removed %d containers\n", removedCount)
	} else {
		fmt.Printf("  ✓ No containers to remove\n")
	}

	return nil
}

// PruneAllContainers removes all yourpm containers
func (p *PruneService) PruneAllContainers() error {
	// Get all yourpm containers
	filters := map[string]string{"name": "yourpm-"}
	containers, err := p.dockerClient.ListContainers(filters)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Printf("  ✓ No yourpm containers found\n")
		return nil
	}

	removedCount := 0

	for _, container := range containers {
		fmt.Printf("  🗑️  Removing container: %s\n", container.Name)
		if err := p.dockerClient.Remove(container.Name); err != nil {
			fmt.Printf("     ⚠️  Failed to remove %s: %v\n", container.Name, err)
		} else {
			removedCount++
		}
	}

	if removedCount > 0 {
		fmt.Printf("  ✓ Removed %d containers\n", removedCount)
	} else {
		fmt.Printf("  ✓ No containers to remove\n")
	}

	return nil
}

// PruneImages removes unused Docker images
func (p *PruneService) PruneImages(aggressive bool) error {
	if aggressive {
		fmt.Printf("  🗑️  Removing all unused images...\n")
	} else {
		fmt.Printf("  🗑️  Removing dangling images...\n")
	}

	return p.dockerClient.PruneImages(aggressive)
}