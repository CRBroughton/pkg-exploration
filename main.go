package main

import (
	"context"

	"github.com/crbroughton/pkg-exploration/pkg/repository"
)

func main() {
	ctx := context.Background()

	githubRepo := repository.NewGithubRepository("./test")

	githubRepo.DownloadFile(ctx, "some-url", "tmp")

}
