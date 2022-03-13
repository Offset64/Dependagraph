package dependagraph

type GithubRepositoryReference struct {
	org  string
	repo string
}

type Repository struct {
	FQN, Organization, Repository, URL, Version, Language string
}
