package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/machinebox/graphql"
)

type rateLimiter struct {
	TokensPerPeriod int
	Duration        time.Duration
	wg              sync.WaitGroup
	t               *time.Ticker
}

func (rl *rateLimiter) Start() {
	if rl.t != nil {
		return
	}

	rl.t = time.NewTicker(rl.Duration)
	go func() {
		<-rl.t.C
		rl.wg.Add(rl.TokensPerPeriod)
	}()
}

func (rl *rateLimiter) Wait(ctx context.Context) error {
	done := make(chan struct{}, 1)
	go func() {
		rl.wg.Wait()
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

var (
	limitDependentPage     = rateLimiter{TokensPerPeriod: 120, Duration: 60 * time.Second}
	limitFetchDependencies = rateLimiter{TokensPerPeriod: 60, Duration: 60 * time.Second}
)

func init() {
	limitDependentPage.Start()
	limitFetchDependencies.Start()
}

type GithubRepositoryReference struct {
	org  string
	repo string
}

func (r GithubRepositoryReference) String() string {
	return strings.Join([]string{r.org, r.repo}, "/")
}

func ParseGithubRepositoryReference(str string) (GithubRepositoryReference, error) {
	parts := strings.Split(str, "/")
	if len(parts) != 2 {
		return GithubRepositoryReference{}, errors.New("must have exactly one slash")
	}

	return GithubRepositoryReference{
		org:  parts[0],
		repo: parts[1],
	}, nil
}

type Repository struct {
	FQN, Organization, Repository, URL, Version, Language string
}

type GithubDependencyScraper struct {
	client          *graphql.Client
	githubAPISecret string
}

// GetDependencies queries Githubs GraphQL endpoint to return a set of all dependencies that this repository depends upon.
func (g *GithubDependencyScraper) GetDependencies(ctx context.Context, ref GithubRepositoryReference) ([]Repository, error) {
	if err := limitFetchDependencies.Wait(ctx); err != nil {
		return nil, err
	}

	req := graphql.NewRequest(`
	query GetDependencies($org: String!, $name: String!) {
			repository(owner: $org, name: $name) {
					dependencyGraphManifests {
							edges {
									node {
									blobPath
									dependencies {
													nodes {
															packageName
															requirements
													}
											}
									}
							}
					}
			}
	}`)
	req.Var("org", ref.org)
	req.Var("name", ref.repo)
	req.Header.Set("Accept", "application/vnd.github.hawkgirl-preview+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", g.githubAPISecret))

	return nil, nil
}

func (g *GithubDependencyScraper) GetDependents(ctx context.Context, ref GithubRepositoryReference) ([]Repository, error) {
	if err := limitDependentPage.Wait(ctx); err != nil {
		return nil, err
	}

	panic("unimplemented!")
}
