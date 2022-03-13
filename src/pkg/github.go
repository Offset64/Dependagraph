package dependagraph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/machinebox/graphql"
)

var (
	limitDependentPage     = newRateLimiter(120, 60*time.Second)
	limitFetchDependencies = newRateLimiter(60, 60*time.Second)
)

type rateLimiter struct {
	t *time.Ticker
	// requestTokens is a channel which contains a number of elements, each representing a transaction that the user can make.
	// The caller can read from this channel to wait for a request to become available.
	// The actual value of this channel  should be thrown away.
	requestTokens chan struct{}
}

func newRateLimiter(tokensPerPeriod int, period time.Duration) rateLimiter {
	rl := rateLimiter{
		t:             time.NewTicker(period),
		requestTokens: make(chan struct{}, tokensPerPeriod),
	}

	go func() {
		for {
			_, ok := <-rl.t.C
			// Ticker was closed.
			if !ok {
				break
			}

		tokenSend:
			for i := 0; i < tokensPerPeriod; i++ {
				// If the channel is full, then we do not want to get 'stuck' in this loop waiting on a send because we will miss future ticks.
				// The rate limit will not "carry over" to new periods, so we use this select statement to discard any superfluous request tokens.
				select {
				case rl.requestTokens <- struct{}{}:
				default:
					break tokenSend
				}
			}
		}

	}()

	return rl
}

func (rl *rateLimiter) Close() {
	rl.t.Stop()
	close(rl.requestTokens)
}

func (rl *rateLimiter) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-rl.requestTokens:
		return nil
	}
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

func NewGithubDependencyScraper(secret string) GithubDependencyScraper {
	return GithubDependencyScraper{
		client:          graphql.NewClient("https://api.github.com/graphql"),
		githubAPISecret: secret,
	}
}

func (g GithubDependencyScraper) prepareRequest(req *graphql.Request) {
	req.Header.Set("Accept", "application/vnd.github.hawkgirl-preview+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", g.githubAPISecret))
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
	g.prepareRequest(req)

	var resp struct {
		Repository struct {
			DependencyGraphManifests struct {
				Edges []struct {
					Node struct {
						BlobPath     string
						Dependencies struct {
							Nodes []struct {
								PackageName, Requirements string
							}
						}
					}
				}
			}
		}
	}

	if err := g.client.Run(ctx, req, &resp); err != nil {
		return nil, err
	}

	var deps []Repository
	for _, edge := range resp.Repository.DependencyGraphManifests.Edges {
		if strings.HasPrefix(edge.Node.BlobPath, ".github/workflows") {
			continue
		}

		for _, dep := range edge.Node.Dependencies.Nodes {
			rep := Repository{
				FQN:      dep.PackageName,
				URL:      dep.PackageName,
				Version:  dep.Requirements,
				Language: "",
			}

			if strings.Contains(dep.PackageName, "github") {
				// dep.Package name will look like github.com/offset64/EOS
				parts := strings.Split(dep.PackageName, "/")
				rep.FQN = fmt.Sprintf("%s/%s", parts[1], parts[2])
				rep.Organization = parts[1]
				rep.Repository = parts[2]
			}

			deps = append(deps, rep)
		}
	}

	return deps, nil
}

func (g *GithubDependencyScraper) GetDependents(ctx context.Context, ref GithubRepositoryReference) ([]Repository, error) {
	if err := limitDependentPage.Wait(ctx); err != nil {
		return nil, err
	}

	panic("unimplemented!")
}
