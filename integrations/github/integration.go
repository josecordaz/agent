package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/pinpt/agent.next/pkg/structmarshal"

	"github.com/pinpt/go-common/hash"

	"github.com/hashicorp/go-hclog"
	"github.com/pinpt/agent.next/integrations/github/api"
	"github.com/pinpt/agent.next/integrations/pkg/ibase"
	"github.com/pinpt/agent.next/rpcdef"
)

func main() {
	ibase.MainFunc(func(logger hclog.Logger) rpcdef.Integration {
		return NewIntegration(logger)
	})
}

type Integration struct {
	logger     hclog.Logger
	agent      rpcdef.Agent
	customerID string

	qc    api.QueryContext
	users *Users

	config Config

	requestConcurrencyChan chan bool
}

func NewIntegration(logger hclog.Logger) *Integration {
	s := &Integration{}
	s.logger = logger
	return s
}

// setting higher to 1 starts returning the following error, even though the hourly limit is not used up yet.
// 403: You have triggered an abuse detection mechanism. Please wait a few minutes before you try again.
const maxRequestConcurrency = 1

func (s *Integration) Init(agent rpcdef.Agent) error {
	s.agent = agent

	qc := api.QueryContext{}
	qc.Logger = s.logger
	qc.Request = s.makeRequest
	qc.CustomerID = s.customerID
	qc.RepoID = func(refID string) string {
		return hash.Values("Repo", s.customerID, "sourcecode.Repo", refID)
	}
	qc.UserID = func(refID string) string {
		return hash.Values("User", s.customerID, "sourcecode.User", refID)
	}
	qc.PullRequestID = func(refID string) string {
		return hash.Values("PullRequest", s.customerID, "sourcecode.PullRequest", refID)
	}
	s.qc = qc
	s.requestConcurrencyChan = make(chan bool, maxRequestConcurrency)

	return nil
}

type Config struct {
	APIURL        string
	RepoURLPrefix string
	Token         string
	OnlyOrg       string
	ExcludedRepos []string
	OnlyRipsrc    bool
}

type configDef struct {
	URL           string   `json:"url"`
	APIToken      string   `json:"apitoken"`
	ExcludedRepos []string `json:"excluded_repos"`
	OnlyRipsrc    bool     `json:"only_ripsrc"`
	// OnlyOrganization specifies the organization to export. By default all account organization are exported. Set this to export only one.
	OnlyOrganization string `json:"only_organization"`
}

func (s *Integration) setIntegrationConfig(data map[string]interface{}) error {
	rerr := func(msg string, args ...interface{}) error {
		return fmt.Errorf("config validation error: "+msg, args...)
	}
	var def configDef
	err := structmarshal.MapToStruct(data, &def)
	if err != nil {
		return err
	}

	if def.URL == "" {
		return rerr("url is missing")
	}
	if def.APIToken == "" {
		return rerr("apitoken is missing")
	}
	var res Config
	res.Token = def.APIToken
	res.OnlyOrg = def.OnlyOrganization
	res.ExcludedRepos = def.ExcludedRepos
	res.OnlyRipsrc = def.OnlyRipsrc

	apiURLBaseParsed, err := url.Parse(def.URL)
	if err != nil {
		return rerr("url is invalid: %v", err)
	}
	res.APIURL = urlAppend(def.URL, "graphql")
	res.RepoURLPrefix = "https://" + strings.TrimPrefix(apiURLBaseParsed.Host, "api.")

	s.config = res
	return nil
}

func (s *Integration) ValidateConfig(ctx context.Context,
	exportConfig rpcdef.ExportConfig) (res rpcdef.ValidationResult, _ error) {

	rerr := func(err error) {
		res.Errors = append(res.Errors, err.Error())
	}

	err := s.initWithConfig(exportConfig)
	if err != nil {
		rerr(err)
		return
	}

	orgs, err := s.getOrgs()
	if err != nil {
		rerr(err)
		return
	}

	_, err = api.ReposAllSlice(s.qc, orgs[0])
	if err != nil {
		rerr(err)
		return
	}

	// TODO: return a repo and validate repo that repo can be cloned in agent

	return
}

func urlAppend(p1, p2 string) string {
	return strings.TrimSuffix(p1, "/") + "/" + p2
}

func (s *Integration) initWithConfig(exportConfig rpcdef.ExportConfig) error {
	s.customerID = exportConfig.Pinpoint.CustomerID
	err := s.setIntegrationConfig(exportConfig.Integration)
	if err != nil {
		return err
	}
	return nil
}

func (s *Integration) Export(ctx context.Context,
	exportConfig rpcdef.ExportConfig) (res rpcdef.ExportResult, _ error) {
	err := s.initWithConfig(exportConfig)
	if err != nil {
		return res, err
	}

	err = s.export(ctx)
	if err != nil {
		return res, err
	}

	return res, nil
}

func (s *Integration) getOrgs() ([]api.Org, error) {
	if s.config.OnlyOrg != "" {
		s.logger.Info("only_organization passed", "org", s.config.OnlyOrg)
		return []api.Org{{Login: s.config.OnlyOrg}}, nil
	}
	res, err := api.OrgsAll(s.qc)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, errors.New("no organizations found in account")
	}
	var names []string
	for _, org := range res {
		names = append(names, org.Login)

	}
	s.logger.Info("found organizations", "orgs", res)
	return res, nil
}

func (s *Integration) export(ctx context.Context) error {
	orgs, err := s.getOrgs()
	if err != nil {
		return err
	}

	// export all users in all organization, and when later encountering new users continue export
	s.users, err = NewUsers(s, orgs)
	if err != nil {
		return err
	}
	defer s.users.Done()

	s.qc.UserLoginToRefID = s.users.LoginToRefID
	s.qc.UserLoginToRefIDFromCommit = s.users.LoginToRefIDFromCommit

	for _, org := range orgs {
		err := s.exportOrganization(ctx, org)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Integration) exportOrganization(ctx context.Context, org api.Org) error {
	s.logger.Info("exporting organization", "login", org.Login)
	repos, err := api.ReposAllSlice(s.qc, org)
	if err != nil {
		return err
	}

	{
		all := map[string]bool{}
		for _, repo := range repos {
			all[repo.ID] = true
		}
		excluded := map[string]bool{}
		for _, id := range s.config.ExcludedRepos {
			if !all[id] {
				return fmt.Errorf("wanted to exclude non existing repo: %v", id)
			}
			excluded[id] = true
		}

		filtered := map[string]api.Repo{}
		// filter excluded repos
		for _, repo := range repos {
			if excluded[repo.ID] {
				continue
			}
			filtered[repo.ID] = repo
		}

		s.logger.Info("repos", "found", len(repos), "excluded_definition", len(s.config.ExcludedRepos), "result", len(filtered))
		repos = []api.Repo{}
		for _, repo := range filtered {
			repos = append(repos, repo)
		}
	}

	// queue repos for processing with ripsrc
	{

		for _, repo := range repos {
			u, err := url.Parse(s.config.RepoURLPrefix)
			if err != nil {
				return err
			}
			u.User = url.UserPassword(s.config.Token, "")
			u.Path = repo.NameWithOwner
			repoURL := u.String()

			args := rpcdef.GitRepoFetch{}
			args.RepoID = s.qc.RepoID(repo.ID)
			args.URL = repoURL
			s.agent.ExportGitRepo(args)
		}
	}

	if s.config.OnlyRipsrc {
		s.logger.Warn("only_ripsrc flag passed, skipping export of data from github api")
		return nil
	}

	// export repos
	{
		err := s.exportRepos(ctx, org, s.config.ExcludedRepos)
		if err != nil {
			return err
		}
	}

	// export a link between commit and github user
	// This is much slower than the rest
	// for pinpoint takes 3.5m for initial, 47s for incremental
	{
		// higher concurrency does not make any real difference
		commitConcurrency := 1

		err := s.exportCommitUsers(repos, commitConcurrency)
		if err != nil {
			return err
		}
	}

	// at the same time, export updated pull requests
	pullRequests := make(chan []api.PullRequest, 10)
	go func() {
		defer close(pullRequests)
		err := s.exportPullRequests(repos, pullRequests)
		if err != nil {
			panic(err)
		}
	}()

	//for range pullRequests {
	//}
	//return nil

	pullRequestsForComments := make(chan []api.PullRequest, 10)
	pullRequestsForReviews := make(chan []api.PullRequest, 10)

	go func() {
		for item := range pullRequests {
			pullRequestsForComments <- item
			pullRequestsForReviews <- item
		}
		close(pullRequestsForComments)
		close(pullRequestsForReviews)
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	// at the same time, export all comments for updated pull requests
	go func() {
		defer wg.Done()
		err := s.exportPullRequestComments(pullRequestsForComments)
		if err != nil {
			panic(err)
		}
	}()
	// at the same time, export all reviews for updated pull requests
	go func() {
		defer wg.Done()
		err := s.exportPullRequestReviews(pullRequestsForReviews)
		if err != nil {
			panic(err)
		}
	}()
	wg.Wait()
	return nil
}