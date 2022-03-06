from shared import Repository
from typing import Tuple, Set

from bs4 import BeautifulSoup
from gql import gql, Client
from gql.transport.aiohttp import AIOHTTPTransport
from ratelimiter import RateLimiter

import logging
import requests
import os


# The opt-in header for dependency graph
GITHUB_GRAPHQL_HEADER = "application/vnd.github.hawkgirl-preview+json"


class DependencyScraper:

    def __init__(self, github_api_secret) -> None:
        self.github_api_secret = github_api_secret

    def get_dependents(self, full_repo_name: Repository) -> Set[Repository]:
        """
        When given a full repository name, e.g. "offset64/EOS",
        it will fetch the dependents from that repository and return them as a list
        """
        dependents_strings = set()  # This will contain the set of strings we find
        target_url = f"https://github.com/{full_repo_name.full_name}/network/dependents"
        while target_url is not None:
            logging.info(f"Loading: {target_url}")
            d, target_url = self._get_dependent_page(target_url)
            dependents_strings.update(d)

        dependents = set()  # We need to convert these strings into a useable Repository
        for d in dependents_strings:
            # TODO: use more than just full_name
            dependents.add(Repository(d, "", "", "", "", ""))
        return dependents

    @RateLimiter(max_calls=120, period=60)  # 120 calls per minute
    def _get_dependent_page(self, url: str) -> Tuple[list, str]:
        """
        Takes a github url, fetches the dependencies from it
        This returns a list containing the dependencies and potentially a string
        denoting the next URL to hit.
        """

        next_page = None
        # The dependents on the page
        dep_selector = "a[data-hovercard-type=repository]"
        # The "Next" button
        page_selector = ".paginate-container .BtnGroup :last-child"

        page = requests.get(url).text

        soup = BeautifulSoup(page, "html.parser")

        # Get the dependents, and strip the leading "/"
        results = [
            r["href"][1:] for r in soup.select(dep_selector)
        ]
        next_page = soup.select_one(page_selector)

        if next_page is not None:
            next_page = None if not "href" in next_page.attrs else next_page["href"]

        return results, next_page

    @RateLimiter(max_calls=60, period=60)  # 60 calls per minute
    def get_dependencies(self, full_repo_name: Repository) -> set:
        """
        Takes a full repo name e.g "offset64/EOS" 
        and queries github's graphql endpoint to return a set of
        dependencies
        """
        org, name = full_repo_name.full_name.split('/')
        query_params = {"org": org, "name": name}

        query = gql(
            """
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
            }
            """
        )

        headers = {"Accept": GITHUB_GRAPHQL_HEADER,
                   "Authorization": f"Bearer {self.github_api_secret}"}

        transport = AIOHTTPTransport(
            "https://api.github.com/graphql", headers=headers)

        with open('schema.docs.graphql', 'r') as dep_file:
            schema_str = dep_file.read()
        client = Client(transport=transport, schema=schema_str)

        logging.info(f"Querying dependencies for: {full_repo_name}")

        results = client.execute(query, variable_values=query_params)

        # a dependency file is what github uses to build its dependency graph
        # e.g:  go.mod, requirements.txt, etc
        dependency_files = results["repository"]["dependencyGraphManifests"]["edges"]

        deps = set()

        for f in dependency_files:
            # Unpack from the graph the node which represents our dependency file
            dep_file = f["node"]
            # Filter out the dependencies the github actions are using.
            # We don't have anything useful to do with them right now
            if not ".github/workflows" in dep_file["blobPath"]:
                for dep in dep_file["dependencies"]["nodes"]:
                    # If we can parse out the github parts, lets try
                    # Otherwise, we'll just use default values for now
                    if "github" in dep["packageName"]:
                        names = dep["packageName"].split('/')
                        # names will look like ["github.com","offset64","EOS"]
                        dep_full_name = names[-2] + "/" + \
                            names[-1]  # "offset64/EOS"
                        dep_org = names[-2]  # "offset64"
                        dep_name = names[-1]  # "EOS"
                    else:
                        dep_full_name = dep["packageName"]
                        dep_org = ""
                        dep_name = ""

                    deps.add(Repository(dep_full_name, dep_org,
                                        dep_name, dep["packageName"], dep["requirements"], ""))

        return deps
