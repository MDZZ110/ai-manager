package model

import (
	"context"
)

type GitLfsCloneOptions struct {
	// The (possibly remote) repository URL to clone from.
	URL string `json:"URL"`
}

type GitLfsFetcher struct {
}

func (gl *GitLfsFetcher) Fetch(ctx context.Context) error {

}
