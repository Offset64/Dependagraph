package dependagraph

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"github.com/machinebox/graphql"
)

var (
	limitFetchDependencies = newRateLimiter(1, 1*time.Second)
	limitFetchDependents   = 1 * time.Second // Colly uses this when constructing the collector
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
func (g *GithubDependencyScraper) GetDependencies(ctx context.Context, ref Repository) ([]Repository, error) {
	log.Printf("Getting dependencies for %+v", ref)
	if err := limitFetchDependencies.Wait(ctx); err != nil {
		return nil, err
	}
	if !ref.InGithub {
		return nil, fmt.Errorf("%s may not be in github. Aborting graphql lookup", ref)
	}

	// TODO: This schema needs to be updated to fetch the dependency repository URL.
	// Right now we can only easily crawl golang projects because we rely on the github.com/org/repo convention
	req := graphql.NewRequest(`
	query GetDependencies($org: String!, $name: String!) {
		repository(owner: $org, name: $name) {
		  dependencyGraphManifests {
			edges {
			  node {
				blobPath # Path of the dependency file which was parsed to generate these results
				dependencies {
				  nodes { # The dependencies
					packageName # How it's named in the package manager
					requirements # The version
					repository{ # The repo that represents the dependency
					  name 
					  url
					  primaryLanguage{
						name
					  }
					}
				  }
				}
			  }
			}
		  }
		}
	  }
	  `)

	var org, repo, err = ref.GithubComponents()
	if err != nil {
		return nil, err
	}
	req.Var("org", org)
	req.Var("name", repo)
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
								Repository                struct {
									Name     string
									URL      string
									Language struct {
										Name string
									}
								}
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
		if strings.Contains(edge.Node.BlobPath, ".github/workflows") {
			continue
		}

		for _, dep := range edge.Node.Dependencies.Nodes {
			var rep Repository
			// If we have the URL, let's use that.
			if dep.Repository.URL != "" {
				rep = NewRepository(dep.Repository.URL)
			} else {
				rep = NewRepository(dep.PackageName)
			}

			rep.Language = dep.Repository.Language.Name
			// TODO: Solve the version normalization problem before adding versions to the database
			// rep.Version = dep.Requirements

			deps = append(deps, rep)
		}
	}

	return deps, nil
}

func (g *GithubDependencyScraper) GetDependents(ctx context.Context, ref Repository) ([]Repository, error) {

	// Initial url to hit
	var initialPage string = fmt.Sprintf("https://github.com/%s/network/dependents", ref)

	var dependents []Repository

	c := colly.NewCollector(
		colly.AllowedDomains("github.com"),
		colly.MaxDepth(1),
	)

	err := c.Limit(&colly.LimitRule{
		DomainGlob: "*",
		Delay:      limitFetchDependents,
	})
	if err != nil {
		return nil, err
	}

	c.OnRequest(func(r *colly.Request) {
		log.Printf("Hitting %s", r.URL)
	})

	//For each dependent we find
	c.OnHTML("a[data-hovercard-type=repository]", func(e *colly.HTMLElement) {
		// The href is likey to be something like "Offset64/dependagraph"
		// so we create a repository from the full URL e.g "https://github.com/Offset64/dependagraph"
		var dependent = NewRepository(
			e.Request.AbsoluteURL(e.Attr("href")),
		)
		dependents = append(dependents, dependent)
	})

	//Follow the "Next" page button
	c.OnHTML(".paginate-container .BtnGroup :last-child", func(e *colly.HTMLElement) {
		//c.Visit()
		c.Visit(e.Attr("href"))

	})

	c.OnError(func(r *colly.Response, err error) {
		log.Fatalf("Encountered error while scraping dependents: %s", err)
	})

	c.Visit(initialPage)

	return dependents, nil
}
