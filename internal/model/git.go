package model

import (
	"fmt"
	"github.com/MDZZ110/ai-manager/internal/constants"
	"github.com/go-git/go-git/v5"
	"strings"
)

type GitCloneOptions struct {
	// The (possibly remote) repository URL to clone from.
	URL string `yaml:"URL"`
	// GB
	LayerSizeThreshold int `yaml:"layerSizeThreshold"`
}

type GitFetcher struct {
	URL string
}

func (gf *GitFetcher) Fetch() (err error) {
	r, err := git.PlainClone(constants.DefaultModelRepoPath, false, &git.CloneOptions{URL: gf.URL})
	if err != nil {
		return
	}

	head, err := r.Head()
	if err != nil {
		return
	}

	branchName := head.Name()
	if !branchName.IsBranch() {
		err = fmt.Errorf("fetch model from git failed, %s is not branch", branchName)
		return
	}

	hash, err := r.ResolveRevision("HEAD")
	if err != nil {
		return
	}

	commitID := hash.String()
	if len(commitID) < 7 {
		err = fmt.Errorf("fetch model from git failed, commitID [%s] invalid", commitID)
	}

	return
}

func getCombinedName(branchName, commitID string) string {
	shortBranchName := strings.Replace(branchName, "refs/tags/", "", 1)
	shortCommitID := commitID[:7]
	return fmt.Sprintf("%s-%s", shortBranchName, shortCommitID)
}
