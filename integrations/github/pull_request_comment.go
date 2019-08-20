package main

import (
	"github.com/pinpt/agent.next/integrations/github/api"
	"github.com/pinpt/agent.next/pkg/objsender"
)

func (s *Integration) exportPullRequestComments(pullRequests chan []api.PullRequest) error {
	sender := objsender.NewNotIncremental(s.agent, "sourcecode.pull_request_comment")
	defer sender.Done()

	for prs := range pullRequests {
		for _, pr := range prs {
			if !pr.HasComments {
				// perf optimization
				continue
			}
			err := s.exportPullRequestCommentsPR(sender, pr.RefID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Integration) exportPullRequestCommentsPR(sender *objsender.NotIncremental, prID string) error {
	return api.PaginateRegular(func(query string) (api.PageInfo, error) {
		pi, res, err := api.PullRequestCommentsPage(s.qc, prID, query)
		if err != nil {
			return pi, err
		}

		var batch []objsender.Model
		//var ids []string
		for _, obj := range res {
			//ids = append(ids, obj.ID)
			batch = append(batch, obj)
		}
		//resIDs <- ids
		return pi, sender.Send(batch)
	})
}