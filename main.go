package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

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

type Neo4jService struct {
	session neo4j.Session
}

func (n *Neo4jService) SaveWindow(ctx context.Context, ref GithubRepositoryReference, dependencies []Repository, dependents []Repository) error {
	_, err := n.session.WriteTransaction(func(tx neo4j.Transaction) (interface{}, error) {
		// Update the reference itself
		nodeID, err := tx.Run("MERGE (c:Repository {full_name: $full_name}) SET c.last_targeted = timestamp() RETURN c", map[string]interface{}{
			"full_name": ref.String(),
		})

		if err != nil {
			return nil, err
		}

		for _, dep := range dependencies {
			v := map[string]interface{}{
				"full_name": dep.FQN,
				"cid":       nodeID,
			}

			tx.Run("MATCH (c) WHERE id(c) = $cid WITH c MERGE (c)-[:DEPENDS_ON]->(r:Repository {full_name: $full_name})", v)
		}

		for _, dep := range dependents {
			v := map[string]interface{}{
				"full_name": dep.FQN,
				"cid":       nodeID,
			}

			tx.Run("MATCH (c) WHERE id(c) = $cid WITH c MERGE (c)<-[:DEPENDS_ON]-(r:Repository {full_name: $full_name})", v)
		}

		return nil, nil
	})

	return err
}

func (n *Neo4jService) GetUntargetedNode() (GithubRepositoryReference, bool) {
	result, err := n.session.ReadTransaction(func(tx neo4j.Transaction) (interface{}, error) {
		result, err := tx.Run("MATCH (n:Repository) WHERE n.last_targeted IS NULL AND NOT n.full_name CONTAINS '.' RETURN n.org, n.name LIMIT 1", nil)
		if err != nil {
			return nil, err
		}

		rec, err := result.Single()
		if err != nil {
			return nil, err
		}

		org, _ := rec.Get("org")
		name, _ := rec.Get("name")

		return GithubRepositoryReference{org: org.(string), repo: name.(string)}, nil
	})

	if err != nil {
		return GithubRepositoryReference{}, false
	}

	repo := result.(*Repository)
	return GithubRepositoryReference{org: repo.Organization, repo: repo.Repository}, true
}

func (n *Neo4jService) Close() {
	n.session.Close()
}

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
