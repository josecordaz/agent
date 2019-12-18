package api

import (
	"time"

	"github.com/pinpt/agent/pkg/date"
	"github.com/pinpt/integration-sdk/sourcecode"
)

func PullRequestReviewsPage(
	qc QueryContext,
	pullRequestRefID string,
	queryParams string) (pi PageInfo, res []*sourcecode.PullRequestReview, totalCount int, rerr error) {

	if pullRequestRefID == "" {
		panic("missing pr id")
	}

	qc.Logger.Debug("pull_request_reviews request", "pr", pullRequestRefID, "q", queryParams)

	query := `
	query {
		node (id: "` + pullRequestRefID + `") {
			... on PullRequest {
				reviews(` + queryParams + `) {
					totalCount
					pageInfo {
						hasNextPage
						endCursor
						hasPreviousPage
						startCursor
					}
					nodes {
						updatedAt
						id
						url
						pullRequest {
							id
						}
						repository {
							id
						}
						state
						createdAt
						author {
							login
						}
					}
				}
			}
		}
	}
	`

	var requestRes struct {
		Data struct {
			Node struct {
				Reviews struct {
					TotalCount int      `json:"totalCount"`
					PageInfo   PageInfo `json:"pageInfo"`
					Nodes      []struct {
						UpdatedAt   time.Time `json:"updatedAt"`
						ID          string    `json:"id"`
						URL         string    `json:"url"`
						PullRequest struct {
							ID string `json:"id"`
						} `json:"pullRequest"`
						Repository struct {
							ID string `json:"id"`
						} `json:"repository"`
						// PENDING,COMMENTED,APPROVED,CHANGES_REQUESTED or DISMISSED
						State     string    `json:"state"`
						CreatedAt time.Time `json:"createdAt"`
						Author    struct {
							Login string `json:"login"`
						}
					} `json:"nodes"`
				} `json:"reviews"`
			} `json:"node"`
		} `json:"data"`
	}

	err := qc.Request(query, &requestRes)
	if err != nil {
		rerr = err
		return
	}

	//qc.Logger.Info(fmt.Sprintf("%+v", res))

	nodesContainer := requestRes.Data.Node.Reviews
	nodes := nodesContainer.Nodes
	//qc.Logger.Info("got reviews", "n", len(nodes))
	for _, data := range nodes {
		item := &sourcecode.PullRequestReview{}
		item.CustomerID = qc.CustomerID
		item.RefType = "github"
		item.RefID = data.ID
		item.URL = data.URL
		//item.UpdatedAt = data.UpdatedAt.Unix()
		item.RepoID = qc.RepoID(data.Repository.ID)
		item.PullRequestID = qc.PullRequestID(item.RepoID, data.PullRequest.ID)

		switch data.State {
		case "PENDING":
			item.State = sourcecode.PullRequestReviewStatePending
		case "COMMENTED":
			item.State = sourcecode.PullRequestReviewStateCommented
		case "APPROVED":
			item.State = sourcecode.PullRequestReviewStateApproved
		case "CHANGES_REQUESTED":
			item.State = sourcecode.PullRequestReviewStateChangesRequested
		case "DISMISSED":
			item.State = sourcecode.PullRequestReviewStateDismissed
		}

		date.ConvertToModel(data.CreatedAt, &item.CreatedDate)

		{
			login := data.Author.Login
			item.UserRefID, err = qc.UserLoginToRefID(login)
			if err != nil {
				qc.Logger.Error("could not resolve pr review author", "login", login, "review_url", data.URL)
			}
		}
		res = append(res, item)
	}

	return nodesContainer.PageInfo, res, nodesContainer.TotalCount, nil
}
