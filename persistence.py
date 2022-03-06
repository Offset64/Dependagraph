from lib2to3.pytree import Node
import logging
from neo4j import GraphDatabase, Transaction
from typing import Set

import neo4j
from shared import Repository


class PersistenceLayer:
    def __init__(self, uri, user, password) -> None:
        self.driver = GraphDatabase.driver(uri, auth=(user, password))

    def close(self):
        self.driver.close()

    def save_window(self, center: Repository, dependencies: Set[Repository], dependents: Set[Repository]):
        logging.info("Saving window")

        with self.driver.session() as session:
            # First we write the "center" of this window. e.g the target
            center_id = session.write_transaction(
                self._update_center, center)
            session.write_transaction(
                self._update_dependencies, center_id, dependencies)
            session.write_transaction(
                self._update_dependents, center_id, dependents)

    def get_untargeted_node(self) -> Repository:
        with self.driver.session() as session:
            return session.read_transaction(self._get_untargeted_node)

    def _get_untargeted_node(self, tx: Transaction) -> Repository:
        # This query intentionally omits any name containing a "."
        # because it may be pointing to something not in github, which we can't crawl (yet)
        query = """
        MATCH (n:Repository) 
        WHERE n.last_targeted IS NULL AND NOT n.full_name CONTAINS '.'
        RETURN n.full_name limit 1
        """
        result = tx.run(query)
        name = result.single()[0]
        return Repository(name, "", "", "", "", "")

    def _update_center(self, tx: Transaction, node: Repository) -> int:
        """
        Attempts update an existing node with a pattern.
        If no such node exists, it's instead created
        """
        query = """
        MERGE (c:Repository {full_name: $full_name})
        SET c.last_targeted = timestamp()
        RETURN c
        """
        result = tx.run(query, {"full_name": node.full_name.lower()})
        return result.single()[0].id

    def _update_dependencies(self, tx: Transaction, center_id: int, dependencies: Set[Repository]):
        query = """
        MATCH (c) WHERE id(c) = $cid
        WITH c
        MERGE (c)-[:DEPENDS_ON]->(r: Repository {full_name: $full_name})
        """
        for repo in dependencies:
            tx.run(
                query, {"full_name": repo.full_name.lower(), "cid": center_id})

    def _update_dependents(self, tx: Transaction, center_id: int, dependents: Set[Repository]):
        query = """
        MATCH (c) WHERE id(c) = $cid
        WITH c
        MERGE (c)<-[:DEPENDS_ON]-(r: Repository {full_name: $full_name})
        """
        for repo in dependents:
            tx.run(
                query, {"full_name": repo.full_name.lower(), "cid": center_id})
