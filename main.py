import os

from shared import Repository
from persistence import PersistenceLayer
from deps import DependencyScraper


from gql.transport.exceptions import TransportQueryError

import logging
import argparse

logging.basicConfig(level=logging.INFO)


# Our API key to interact with github
GITHUB_API_SECRET = os.getenv("GITHUB_API_SECRET")
# Database creds
NEO4J_URI = os.getenv("NEO4J_URI")
NEO4J_USR = os.getenv("NEO4J_USR")
NEO4J_PWD = os.getenv("NEO4J_PWD")


def run(target: Repository, db: PersistenceLayer, deps: DependencyScraper):
    logging.info(f"Running against {target.full_name}")

    # TODO: Make this a standalone flag, i.e --dependents
    # to run only the target and its dependents (same for dependencies)
    try:
        dependencies = deps.get_dependencies(target)
        dependents = deps.get_dependents(target)
    except TransportQueryError:
        logging.warning(
            f"{target.full_name} errored on graphql query. Likely isn't in github. Skipping")
        # This run has failed. Dump the data, and let the database mark this node as explored
        dependencies = set()
        dependents = set()

    db.save_window(target, dependencies=dependencies, dependents=dependents)


if __name__ == "__main__":

    parser = argparse.ArgumentParser(
        description="Populate a neo4j database with a dependency graph window",
        epilog="Remember: The graph must grow to support the needs of a growing graph")
    parser.add_argument('repository', metavar='repo', type=str,
                        help='The repo to seed the graph with. Must be in the form of org/repo (e.g. Offset64/dependagraph)')
    parser.add_argument("--coalesce", action="store_true", help="This enables unlimited crawling mode.\
        After seeding, grab a leaf node and run again with the leaf as the new seed.")

    args = parser.parse_args()

    if NEO4J_URI is None:
        raise Exception("NEO4J_URI not set")
    if NEO4J_USR is None:
        raise Exception("NEO4J_USR not set")
    if NEO4J_PWD is None:
        raise Exception("NEO4J_PWD not set")
    if GITHUB_API_SECRET is None:
        raise Exception("GITHUB_API_SECRET not set")

    # make sure the repo is well formatted
    if len(args.repository.split('/')) != 2:
        raise Exception(f"Expected repo: org/repo, got {args.repository}")

    db = PersistenceLayer(NEO4J_URI, NEO4J_USR, NEO4J_PWD)
    deps = DependencyScraper(GITHUB_API_SECRET)

    # TODO: Update this when Repository actually needs more metadata
    target = Repository(args.repository, "", "", "", "", "")
    run(target, db=db, deps=deps)

    if args.coalesce:
        logging.critical("RUNNING IN COALESCE MODE. MAY RUN FOREVER.")
        next_target = db.get_untargeted_node()
        while isinstance(next_target, Repository):
            run(next_target, db=db, deps=deps)
            next_target = db.get_untargeted_node()

    db.close()
