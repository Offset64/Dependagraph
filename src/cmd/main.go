package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	dependagraph "github.com/offset64/dependagraph/pkg"

	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
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

	ctx := context.Background()
	ref, err := dependagraph.ParseGithubRepositoryReference(repository)
	if err != nil {
		log.Fatalf("invalid github repository reference: %s", err)
	}

	scraper := dependagraph.NewGithubDependencyScraper(opts.GithubAPISecret)
	drv, err := neo4j.NewDriver(opts.URI, neo4j.BasicAuth(opts.User, opts.Password, ""))
	if err != nil {
		log.Fatalf("could not establish connection to neo4j: %s", err)
	}

	db := dependagraph.NewNeo4jService(drv)
	defer db.Close()

	type taskResult struct {
		Ref   dependagraph.GithubRepositoryReference
		Error error
	}

	tasks := make(chan taskResult, 1)
	tasks <- taskResult{
		Ref:   ref,
		Error: fetchGithubRepository(ctx, ref, scraper, db),
	}

	if coalesce {
		log.Printf("RUNNING IN COALESCE MODE. MAY RUN FOREVER.")
		go func() {
			// This is not optimally written because GetUntargetedNode will only ever return one result, and repeated calls may return the same result.
			// A 'real' example would modify the below loop to pull many untargeted nodes in parallel, as it is unlikely we will come close to our rate limit like this.
			for {
				ref, ok := db.GetUntargetedNode(ctx)
				if !ok {
					break
				}

				tasks <- taskResult{Ref: ref, Error: fetchGithubRepository(ctx, ref, scraper, db)}
			}

			close(tasks)
		}()
	}

	for task := range tasks {
		if task.Error != nil {
			log.Printf("[%s] failed: %s", task.Ref, task.Error)
		} else {
			log.Printf("[%s] updated", task.Ref)
		}
	}
}

func fetchGithubRepository(ctx context.Context, ref dependagraph.GithubRepositoryReference, scraper dependagraph.GithubDependencyScraper, db dependagraph.Neo4jService) error {
	var wg sync.WaitGroup
	var dependencies, dependents []dependagraph.Repository
	var errs struct {
		dependencies error
		dependents   error
	}

	wg.Add(2)
	// This mess is so we can process both at the same time.
	// This is simpler than using channels.
	go func() {
		dependents, errs.dependents = scraper.GetDependents(ctx, ref)
		wg.Done()
	}()

	go func() {
		dependencies, errs.dependencies = scraper.GetDependencies(ctx, ref)
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
