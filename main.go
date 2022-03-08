package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/machinebox/graphql"
)

type Options struct {
	URI, User, Password, GithubAPISecret string
}

var (
	repository string
	coalesce   bool
)

func init() {
	flag.StringVar(&repository, "repository", "", "The repo to seed the graph with.\nMust be in the form of org/repo (e.g, offset46/Dependagraph)")
	flag.BoolVar(&coalesce, "coalesce", false, "This enables unlimited crawling mode.\nAfter seeding, grab a leaf node and run again with the leaf as the new seed.")
	flag.Parse()
}

func main() {
	opts := Options{
		URI:             os.Getenv("NEO4J_URI"),
		User:            os.Getenv("NEO4J_USR"),
		Password:        os.Getenv("NEO4J_PWD"),
		GithubAPISecret: os.Getenv("GITHUB_API_SECRET"),
	}

	if opts.URI == "" {
		log.Fatalln("NEO4J_URI not set")
	}

	if opts.User == "" {
		log.Fatalln("NEO4J_USR not set")
	}

	if opts.Password == "" {
		log.Fatalln("NEO4J_PWD not set")
	}

	if opts.GithubAPISecret == "" {
		log.Fatalln("GITHUB_API_SECRET not set")
	}

	ref, err := ParseGithubRepositoryReference(repository)
	if err != nil {
		log.Fatalf("invalid github repository reference: %s", err)
	}

	db := Neo4jService{}
	scraper := GithubDependencyScraper{}
	go fetchGithubRepository(context.TODO(), ref, scraper, db)

	defer db.Close()
	if !coalesce {
		return
	}

	log.Printf("RUNNING IN COALESCE MODE. MAY RUN FOREVER.")
	for {
		ref, ok := db.GetUntargetedNode()
		if !ok {
			break
		}

		go fetchGithubRepository(context.TODO(), ref, scraper, db)
	}
}

type Neo4jService struct{}

func (n *Neo4jService) SaveWindow(ctx context.Context, ref GithubRepositoryReference, dependencies []Repository, dependents []Repository) error

func (n *Neo4jService) GetUntargetedNode() (GithubRepositoryReference, bool)

func (n *Neo4jService) Close()

func fetchGithubRepository(ctx context.Context, ref GithubRepositoryReference, scraper GithubDependencyScraper, db Neo4jService) error {
	var wg sync.WaitGroup
	var dependencies, dependents []Repository
	var errs struct {
		dependencies error
		dependents   error
	}

	wg.Add(2)
	// This mess is so we can process both at the same time.
	// This is simpler than using channels.
	go func() {
		dependents, errs.dependents = scraper.GetDependents(context.TODO(), ref)
		wg.Done()
	}()

	go func() {
		dependencies, errs.dependencies = scraper.GetDependencies(context.TODO(), ref)
		wg.Done()
	}()

	wg.Wait()
	if errs.dependencies != nil {
		return fmt.Errorf("failed to fetch dependencies: %w", errs.dependencies)
	}

	if errs.dependents != nil {
		return fmt.Errorf("failed to fetch dependents: %w", errs.dependents)
	}

	return db.SaveWindow(ctx, ref, dependencies, dependents)
}

type GithubRepositoryReference struct {
	org  string
	repo string
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

type GithubDependencyScraper struct {
	client          *graphql.Client
	githubAPISecret string
}

type Repository struct {
	FQN, Organization, Repository, URL, Version, Language string
}

// GetDependencies queries Githubs GraphQL endpoint to return a set of all dependencies that this repository depends upon.
func (g *GithubDependencyScraper) GetDependencies(ctx context.Context, ref GithubRepositoryReference) ([]Repository, error) {
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

func (g *GithubDependencyScraper) GetDependents(ctx context.Context, ref GithubRepositoryReference) ([]Repository, error)
