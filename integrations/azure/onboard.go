package main

import (
	"fmt"
	"time"

	"github.com/pinpt/agent/integrations/azure/api"
	"github.com/pinpt/agent/pkg/date"
	"github.com/pinpt/agent/pkg/ids"
	"github.com/pinpt/agent/rpcdef"
	"github.com/pinpt/integration-sdk/agent"
	"github.com/pinpt/integration-sdk/sourcecode"
)

func (s *Integration) onboardExportRepos() (res rpcdef.OnboardExportResult, err error) {

	var repos []*sourcecode.Repo
	_, repos, err = s.api.FetchAllRepos([]string{}, []string{}, []string{})
	if err != nil {
		s.logger.Error("error fetching repos for onboard export repos")
		return
	}
	var records []map[string]interface{}
	for _, repo := range repos {
		var rawcommit *api.CommitResponse
		rawcommit, err = s.api.FetchLastCommit(repo.RefID)
		if rawcommit == nil {
			s.logger.Debug("last commit is nil, skipping", "ref_id", repo.RefID, "name", repo.Name)
			continue
		}
		if err != nil {
			s.logger.Error("error fetching last commit for onboard, skipping", "repo ref_id", repo.RefID, "err", err)
			return
		}
		r := &agent.RepoResponseRepos{
			Active:      repo.Active,
			Description: repo.Description,
			Language:    repo.Language,
			LastCommit: agent.RepoResponseReposLastCommit{
				Author: agent.RepoResponseReposLastCommitAuthor{
					Name:  rawcommit.Author.Name,
					Email: rawcommit.Author.Email,
				},
				CommitSha: rawcommit.CommitID,
				CommitID:  ids.CodeCommit(s.customerid, s.RefType.String(), repo.ID, rawcommit.CommitID),
				URL:       rawcommit.URL,
				Message:   rawcommit.Comment,
			},
			Name:    repo.Name,
			RefID:   repo.RefID,
			RefType: repo.RefType,
		}
		date.ConvertToModel(rawcommit.Author.Date, &r.LastCommit.CreatedDate)
		records = append(records, r.ToMap())
	}
	res.Data = records
	return
}

func (s *Integration) onboardExportProjects() (res rpcdef.OnboardExportResult, err error) {
	projects, err := s.api.FetchProjects([]string{}, []string{}, []string{})
	var records []map[string]interface{}
	for _, proj := range projects {
		itemids, err := s.api.FetchItemIDs(proj.RefID, time.Time{})
		if err != nil {
			s.logger.Error("error getting issue count", "err", err)
			return res, err
		}
		raw, lastitem, err := s.api.FetchWorkItemsByIDs(proj.RefID, append([]string{}, itemids[len(itemids)-1]))
		if err != nil {
			s.logger.Error("error getting last issue", "err", err)
			return res, err
		}
		resp := &agent.ProjectResponseProjects{
			Active:     proj.Active,
			Identifier: proj.Identifier,
			LastIssue: agent.ProjectResponseProjectsLastIssue{
				IssueID:     ids.WorkIssue(s.customerid, proj.RefType, fmt.Sprintf("%d", raw[0].ID)),
				Identifier:  lastitem[0].Identifier,
				CreatedDate: agent.ProjectResponseProjectsLastIssueCreatedDate(lastitem[0].CreatedDate),
				LastUser: agent.ProjectResponseProjectsLastIssueLastUser{
					UserID:    raw[0].Fields.CreatedBy.ID,
					Name:      raw[0].Fields.CreatedBy.DisplayName,
					AvatarURL: raw[0].Fields.CreatedBy.ImageURL,
				},
			},
			Name:        proj.Name,
			RefID:       proj.RefID,
			RefType:     proj.RefType,
			TotalIssues: int64(len(itemids)),
			URL:         proj.URL,
		}
		records = append(records, resp.ToMap())
	}
	res.Data = records
	return res, err
}

func (s *Integration) onboardWorkConfig() (res rpcdef.OnboardExportResult, err error) {
	conf, err := s.api.FetchWorkConfig()
	if err != nil {
		res.Error = err
		return
	}
	res.Data = conf.ToMap()
	return
}