package model

import "context"

type ModelFetcher interface {
	Fetch(ctx context.Context) error
}

type ModelFetcherOption struct {
	GitURL string
}

func NewModelFetcher(options ModelFetcherOption) ModelFetcher {

}
