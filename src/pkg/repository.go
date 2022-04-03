package dependagraph

import (
	"fmt"
	"strings"
)

// Repository TODO: Figure out if we actually need name + org if it can just be full_name. Then update the db and the code accordingly
type Repository struct {
	Name     string // What GitHub calls it in the dependencyGraphManifests - e.g "dependagraph"
	Org      string // Which organization this belongs to - e.g "offset64"
	URL      string // This is the URL where we can find the package. It's also the primary element we match on when building the database
	Language string // The language of the package - e.g "go"
	Version  string // TODO: Determine how this'll be normalized (e.g "= 3.1.14" vs whatever)
	InGithub bool   // Flag to determine if a repo exists in GitHub, or somewhere else. This way we only crawl what's in GitHub (for now)
}

// NewRepository Construct a new Repository from a name. This name can be in the form of org/repo (e.g "offset64/dependagraph")
// or it can be in the form of a URL, e.g "https://github.com/offset64/dependagraph"
func NewRepository(name string) Repository {
	// It's a GitHub url, we know how to parse it
	// Or it's in the form of "org/repo", which we'll just assume is GitHub
	if isValidGithubName(name) {
		var o, n, err = parseRepoName(name)
		// If we get an org with a "." in it, that means we've got something like gopkg.in, which we don't want to say
		// lives in GitHub
		//TODO: Refactor this part so we can perform all of our name checks in one place, then return the right type of repo
		if err == nil && !strings.Contains(o, ".") {
			return Repository{
				Name:     n,
				Org:      o,
				URL:      fmt.Sprintf("https://github.com/%s/%s", o, n),
				InGithub: true,
			}
		}
	}
	// The repository is something, we just don't know what.
	// e.g "github.com/something/not/expected" or "go.something.com/xyz"
	return Repository{
		Name:     name,
		InGithub: false,
	}
}

//Todo: implement NewRepositoryFromDatabaseRecord

func isValidGithubName(name string) bool {
	return strings.Contains(name, "https://github.com") || strings.Contains(name, "github.com") || len(strings.Split(name, "/")) == 2
}

// GithubComponents For this repository, return a tuple containing the org and the repo
// This is parsed from the URL of the repo. If the repo is not in GitHub
// or otherwise can't be safely parsed into the org/repo format, then
// an error will be returned.
//
// e.g a Repository with a URL of https://github.com/offset64/dependagraph will
// return ("offset64", "dependagraph", nil)
func (r Repository) GithubComponents() (string, string, error) {
	var urlPrefix = "https://github.com/"
	if !strings.Contains(r.URL, urlPrefix) {
		return "", "", fmt.Errorf("%s is not part of %s", r.URL, urlPrefix)
	}
	return parseRepoName(r.URL)
}

func (r Repository) String() string {
	if r.Org != "" {
		return fmt.Sprintf("%s/%s", r.Org, r.Name)
	}
	return r.Name
}

func parseRepoName(name string) (string, string, error) {

	var urlPrefix = "https://github.com/"
	var prefixLen = len(urlPrefix)
	var repo = name
	if strings.Contains(name, urlPrefix) {
		repo = name[prefixLen:]
	}

	var parsed = strings.Split(repo, "/")

	if len(parsed) == 2 { // We've got the org/name as expected
		return parsed[0], parsed[1], nil
	}
	if len(parsed) == 3 && parsed[0] == "github.com" { // We've got github.com/org/name
		return parsed[1], parsed[2], nil
	}
	return "", "", fmt.Errorf("unexpected format for %s", name)
}
