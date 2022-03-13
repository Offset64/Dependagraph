package dependagraph

import (
	"fmt"
	"strings"
)

type Repository struct {
	Name     string // What github calls it in the dependencyGraphManifests - e.g "dependagraph"
	Org      string // Which organization this belongs to - e.g "offset64"
	URL      string // This is the URL where we can find the package. It's also the primary element we match on when building the database
	Language string // The language of the package - e.g "go"
	Version  string // TODO: Determine how this'll be normalized (e.g "= 3.1.14" vs whatever)
	InGithub bool   // Flag to determine if a repo exists in github, or somewhere else. This way we only crawl what's in github (for now)
}

//Construct a new Repository from a name. This name can be in the form of org/repo (e.g "offset64/dependagraph")
// or it can be in the form of a URL, e.g "https://github.com/offset64/dependagraph"
func NewRepository(name string) Repository {

	// It's a github url, we know how to parse it
	// Or it's in the form of "org/repo", which we'll just assume is github
	if strings.Contains(name, "https://github.com") || len(strings.Split(name, "/")) == 2 {
		var o, n, err = parse_repo_name(name)
		if err == nil {
			return Repository{
				Name:     n,
				Org:      o,
				URL:      fmt.Sprintf("https://github.com/%s/%s", o, n),
				InGithub: true,
			}
		}

	}

	// It's a URL but not a github one
	if strings.Contains(name, "https://") {
		return Repository{
			URL:      name,
			InGithub: false,
		}
	}

	// The repository is something, we just don't know what.
	// e.g "github.com/something/not/expected" or "go.something.com/xyz"
	return Repository{
		Name:     name,
		InGithub: false,
	}

}

// For this repository, return a tuple containing the org and the repo
// This is parsed from the URL of the repo. If the repo is not in github
// or otherwise can't be safely parsed into the org/repo format, then
// an error will be returned.
//
// e.g a Repository with a URL of https://github.com/offset64/dependagraph will
// return ("offset64", "dependagraph", nil)
func (r Repository) GithubComponents() (string, string, error) {
	var url_prefix = "https://github.com/"
	if !strings.Contains(r.URL, url_prefix) {
		return "", "", fmt.Errorf("%s is not part of %s", r.URL, url_prefix)
	}
	return parse_repo_name(r.URL)
}

func (r Repository) String() string {
	return fmt.Sprintf("%s/%s", r.Org, r.Name)
}

func parse_repo_name(str string) (string, string, error) {

	var url_prefix = "https://github.com/"
	var prefix_len = len(url_prefix)
	var repo = str
	if strings.Contains(str, url_prefix) {
		repo = str[prefix_len:]
	}

	var parsed = strings.Split(repo, "/")

	if len(parsed) == 2 {
		return parsed[0], parsed[1], nil
	}
	return "", "", fmt.Errorf("unexpected format for %s", str)
}
