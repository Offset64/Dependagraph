package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"strings"
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

func (n *Neo4jService) GetUntargetedNode() (GithubRepositoryReference, bool) {
	return GithubRepositoryReference{}, false
}

func (n *Neo4jService) Close() error {
	return nil
}

func fetchGithubRepository(ctx context.Context, ref GithubRepositoryReference, scraper GithubDependencyScraper, db Neo4jService) {
	// TODO: Get dependents and dependencies and store them in the DB
	go scraper.GetDependents(context.TODO(), ref)
	go scraper.GetDependencies(context.TODO(), ref)
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

type GithubDependencyScraper struct{}

func (g *GithubDependencyScraper) GetDependencies(ctx context.Context, ref GithubRepositoryReference) error {
	return nil
}

func (g *GithubDependencyScraper) GetDependents(ctx context.Context, ref GithubRepositoryReference) error {
	return nil
}
