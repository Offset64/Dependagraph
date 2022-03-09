package main

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
)

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
