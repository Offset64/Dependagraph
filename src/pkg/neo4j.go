package dependagraph

import (
	"context"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j/dbtype"
	"log"
)

type Neo4jService struct {
	drv neo4j.Driver
}

const (
	updateRepoDependsOnQuery = `
MATCH (c) WHERE ID(c) = $cid
MERGE (r:Repository {full_name: $full_name, version: $version, language: $language, in_github: $inGithub})
WITH c, r
MERGE (c)-[:DEPENDS_ON]->(r)`
	updateRepoDependedOnByQuery = `
MATCH (c) WHERE ID(c) = $cid
MERGE (r:Repository {full_name: $full_name, version: $version, language: $language, in_github: $inGithub})
WITH c, r
MERGE (c)<-[:DEPENDS_ON]-(r)`
)

//TODO: Find a better way to unpack the response from neo4j containing a list of results. e.g
//type Neo4jResponse struct {
//	Results []struct {
//		Columns []string `json:"columns"`
//		Data    []struct {
//			Repositories []Repository `json:"row"`
//		} `json:"data"`
//	} `json:"results"`
//	Errors []string `json:"errors"`
//}

func NewNeo4jService(driver neo4j.Driver) Neo4jService {
	return Neo4jService{drv: driver}
}

func (n *Neo4jService) SaveWindow(_ context.Context, ref Repository, dependencies []Repository, dependents []Repository) error {
	// Context is currently a no-op as neo4j does not support it.
	session := n.drv.NewSession(neo4j.SessionConfig{})
	defer session.Close()
	_, err := session.WriteTransaction(func(tx neo4j.Transaction) (interface{}, error) {
		// Update the reference itself
		result, err := tx.Run("MERGE (c:Repository {full_name: $full_name, version: $version, language: $language, in_github: $inGithub}) SET c.last_targeted = timestamp() RETURN c", map[string]interface{}{
			"full_name": ref.String(),
			"version":   ref.Version,
			"language":  ref.Language,
			"inGithub":  ref.InGithub,
		})

		if err != nil {
			return nil, err
		}

		resultRecord, err := result.Single()

		if err != nil {
			log.Printf("WARNING: MERGE for %+v returned multiple IDs", ref)
		}

		centerNodeID := resultRecord.Values[0].(dbtype.Node).Id

		for _, dep := range dependencies {
			v := map[string]interface{}{
				"full_name": dep.String(),
				"cid":       centerNodeID,
				"version":   dep.Version,
				"language":  dep.Language,
				"inGithub":  dep.InGithub,
			}
			tx.Run(updateRepoDependsOnQuery, v)
		}

		for _, dep := range dependents {
			v := map[string]interface{}{
				"full_name": dep.String(),
				"cid":       centerNodeID,
				"version":   dep.Version,
				"language":  dep.Language,
				"inGithub":  dep.InGithub,
			}

			tx.Run(updateRepoDependedOnByQuery, v)
		}

		return nil, nil
	})

	return err
}

// GetUntargetedNodes returns a list of up to 100 nodes which have not yet been scanned. This limit is arbitrary
func (n *Neo4jService) GetUntargetedNodes(_ context.Context) ([]Repository, bool) {
	// Context support is currently a noop because Neo4j does not support it.
	session := n.drv.NewSession(neo4j.SessionConfig{})
	defer session.Close()

	var untargetedNodes []Repository
	_, err := session.ReadTransaction(func(tx neo4j.Transaction) (interface{}, error) {
		const query = "MATCH (n) WHERE n.last_targeted IS NULL AND n.in_github = true RETURN n limit 100"
		result, err := tx.Run(query, nil)
		if err != nil {
			return nil, err
		}

		for result.Next() {
			record := result.Record()
			node, _ := record.Get("n")
			name := node.(dbtype.Node).Props["full_name"].(string)
			untargetedNodes = append(untargetedNodes, NewRepository(name))
		}
		return nil, nil
	})

	if err != nil || len(untargetedNodes) == 0 {
		return nil, false
	}

	return untargetedNodes, true
}

func (n *Neo4jService) Close() {
	n.drv.Close()
}
