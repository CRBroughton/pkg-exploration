# PKG Exploration

Exploring the idea of making a declarative package manager with Go.

You can find an example `config.example.toml` in the repository.

Run `cp manifest.example.toml ~/.yourpm/manifest.toml` to copy the example manifest.

Run `./pkg-exploration switch config.example.toml` to install the
example configuration.

## Commands

### Switch Environment

Switch to a specific configuration, installing packages and setting up containers:

```bash
# Switch to a specific config file
yourpm switch config.example.toml

# Switch using default config (~/.yourpm/config.toml)
yourpm switch
```

### Prune Resources

Clean up unused Docker resources with granular control:

#### Container Pruning

```bash
# Remove containers not in current config (safe)
yourpm prune containers

# Remove ALL yourpm containers (aggressive)
yourpm prune containers --all
```

The basic `prune containers` command intelligently keeps containers that are defined in your current configuration while removing containers from previous configurations that are no longer needed.

#### Image Pruning

```bash
# Remove dangling Docker images only
yourpm prune images

# Remove all unused Docker images (aggressive)
yourpm prune images --all
```

The basic `prune images` command removes only dangling images (untagged intermediate images), while the `--all` flag removes all images not currently used by any containers.

#### Examples

```bash
# Clean up after switching configurations
yourpm switch new-config.toml
yourpm prune containers  # Removes old containers not in new-config.toml

# Deep clean to reclaim maximum space
yourpm prune containers --all
yourpm prune images --all

# Conservative cleanup
yourpm prune containers  # Keep active containers
yourpm prune images      # Remove only dangling images
```

**Note**: The prune commands are designed to be safe by default. Without the `--all` flag, they preserve resources that are actively being used or defined in your current configuration.