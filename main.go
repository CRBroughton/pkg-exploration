package main

import (
	"context"
	"fmt"

	"github.com/crbroughton/pkg-exploration/pkg/repository"
)

func main() {
	ctx := context.Background()

	githubRepo := repository.NewGithubRepository("./tmp")

	err := githubRepo.DownloadFile(ctx, "https://github.com/jqlang/jq/releases/download/jq-1.8.1/jq-linux-amd64", "./tmp/jq")
	if err != nil {
		fmt.Printf("something went wrong: %v\n", err)
	} else {
		fmt.Println("jq binary downloaded")
	}
}
