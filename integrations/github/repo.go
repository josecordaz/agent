package main

import (
	"context"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/pinpt/agent.next/integrations/github/api"
)

func (s *Integration) exportRepos(ctx context.Context, logger hclog.Logger, org api.Org, onlyInclude []api.Repo) error {

	// map[nameWithOwner]shouldInclude
	shouldInclude := map[string]bool{}
	for _, repo := range onlyInclude {
		shouldInclude[repo.NameWithOwner] = true
	}

	sender := s.repoSender

	err := api.PaginateNewerThan(sender.LastProcessed, func(query string, stopOnUpdatedAt time.Time) (api.PageInfo, error) {
		pi, repos, err := api.ReposPage(s.qc.WithLogger(logger), org, query, stopOnUpdatedAt)
		if err != nil {
			return pi, err
		}
		for _, repo := range repos {
			// sourcecode.Repo.Name == api.Repo.NameWithOwner
			if !shouldInclude[repo.Name] {
				continue
			}
			err := sender.Send(repo)
			if err != nil {
				return pi, err
			}
		}
		return pi, nil
	})

	if err != nil {
		return err
	}

	return nil
}
