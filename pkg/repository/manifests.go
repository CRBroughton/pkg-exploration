package repository

func loadGitHubManifests() map[string]*GithubManifest {
	return map[string]*GithubManifest{
		"jq": {
			Repo: "jqlang/jq",
		},
	}
}
