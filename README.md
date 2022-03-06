# Dependagraph

## Help
```bash
python3 main.py -h
```
```
usage: main.py [-h] [--coalesce] repo

Populate a neo4j database with a dependency graph window

positional arguments:
  repo        The repo to seed the graph with. Must be in the form of org/repo (e.g.
              Offset64/dependagraph)

optional arguments:
  -h, --help  show this help message and exit
  --coalesce  This enables unlimited crawling mode. After seeding, grab a leaf node and run again with the
              leaf as the new seed.

Remember: The graph must grow to support the needs of a growing graph

```

## Setup
Set up your environment variables
```bash
export GITHUB_API_SECRET="<Your GitHub API key>"
# Creds for your local or remote neo4j instance 
export NEO4J_URI="bolt://127.0.0.1:7687" 
export NEO4J_USR="neo4j"
export NEO4J_PWD="pass"
```

Install dependencies
```bash
python3 -m pip install -r requirements.txt
```

Run it on a single repo

```bash
python3 main.py Offset64/dependagraph
```

You'll see your neo4j database populated with that repo, its dependencies, and its dependents.

## Longer running process
Run it with `--coalesce` to build the full graph of publicly available repositories. 
This could crawl forever. Keep an eye on your resource consumption.

e.g.

```bash
python3 main.py Offset64/dependagraph --coalesce
```
