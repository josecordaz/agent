package jiracommon

import (
	"net/url"

	"github.com/pinpt/agent.next/integrations/pkg/jiracommonapi"
	"github.com/pinpt/agent.next/rpcdef"

	"github.com/hashicorp/go-hclog"
)

type Opts struct {
	BaseURL          string
	Logger           hclog.Logger
	CustomerID       string
	Request          func(objPath string, params url.Values, res interface{}) error
	Agent            rpcdef.Agent
	ExcludedProjects []string
}

type JiraCommon struct {
	opts  Opts
	agent rpcdef.Agent
	users *Users
}

func New(opts Opts) (*JiraCommon, error) {
	s := &JiraCommon{}
	if opts.BaseURL == "" || opts.CustomerID == "" || opts.Request == nil || opts.Agent == nil {
		panic("provide required params")
	}
	s.opts = opts
	s.agent = opts.Agent
	return s, nil
}

func (s *JiraCommon) SetupUsers() error {
	var err error
	s.users, err = NewUsers(s.opts.CustomerID, s.opts.Agent)
	return err
}

func (s *JiraCommon) CommonQC() jiracommonapi.QueryContext {
	res := jiracommonapi.QueryContext{}
	res.BaseURL = s.opts.BaseURL
	res.CustomerID = s.opts.CustomerID
	res.Logger = s.opts.Logger
	res.ExportUser = s.users.ExportUser
	res.Request = s.opts.Request
	res.Validate()
	return res
}

func (s *JiraCommon) ExportDone() error {
	return s.users.Done()
}
