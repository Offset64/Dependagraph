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
	flag.StringVar(&repository, "repository", "", "The repo to seed the graph with.\nMust be in the form of org/repo (e.g, offset64/Dependagraph) or a github url e.g https://github.com/offset64/dependagraph")
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
	scraper := dependagraph.NewGithubDependencyScraper(opts.GithubAPISecret)
	drv, err := neo4j.NewDriver(opts.URI, neo4j.BasicAuth(opts.User, opts.Password, ""))
	if err != nil {
		log.Fatalf("could not establish connection to neo4j: %s", err)
	}

	db := dependagraph.NewNeo4jService(drv)
	defer db.Close()

	type taskResult struct {
		Ref   dependagraph.Repository
		Error error
	}

	tasks := make(chan taskResult, 100) //arbitrary limit

	if coalesce {
		log.Printf("RUNNING IN COALESCE MODE AND WILL RUN FOREVER")
		//TODO: Fix this up so we can run fetchGithubRepository on multiple things simultaneously while respecting rate limits
		go func() {
			for {
				untargetedNodes, ok := db.GetUntargetedNodes(ctx)
				if !ok {
					break
				}
				for _, n := range untargetedNodes {
					tasks <- taskResult{Ref: n, Error: fetchGithubRepository(ctx, n, scraper, db)}
				}
				log.Println("Finished batch of untargeted nodes. Grabbing new batch")
			}

			close(tasks)
		}()
	} else { // we're only targeting a single repo
		ref := dependagraph.NewRepository(repository)
		if !ref.InGithub {
			log.Fatalf("invalid github repository reference: %s", ref)
		}

		tasks <- taskResult{
			Ref:   ref,
			Error: fetchGithubRepository(ctx, ref, scraper, db),
		}

		close(tasks)
	}

	for task := range tasks {
		if task.Error != nil {
			log.Printf("[%s] failed: %s", task.Ref, task.Error)
		} else {
			log.Printf("[%s] updated", task.Ref)
		}
	}
}

func fetchGithubRepository(ctx context.Context, ref dependagraph.Repository, scraper dependagraph.GithubDependencyScraper, db dependagraph.Neo4jService) error {
	var wg sync.WaitGroup
	var dependencies, dependents []dependagraph.Repository
	var errs struct {
		dependencies error
		dependents   error
	}
	wg.Add(1)
	//This mess is so we can process both at the same time.
	//This is simpler than using channels.
	//TODO: Fix this, it doesn't fetch both simultaneously
	//go func() {
	//	dependents, errs.dependents = scraper.GetDependents(ctx, ref)
	//	wg.Done()
	//}()
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
