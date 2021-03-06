package api

import (
	"fmt"

	"github.com/pinpt/agent/pkg/date"
	"github.com/pinpt/integration-sdk/work"
)

func (api *API) FetchSprint(projid string, teamid string) (sprints []*work.Sprint, err error) {
	url := fmt.Sprintf(`%s/%s/_apis/work/teamsettings/iterations`, projid, teamid)
	var res []sprintsResponse
	if err = api.getRequest(url, nil, &res); err != nil {
		return nil, err
	}
	for _, r := range res {
		sprint := work.Sprint{
			CustomerID: api.customerid,
			// Goal is missing
			Name:    r.Name,
			RefID:   r.Path, // ID's don't match changelog IDs, use path here and IterationPath there
			RefType: api.reftype,
		}
		switch r.Attributes.TimeFrame {
		case "past":
			sprint.Status = work.SprintStatusClosed
		case "current":
			sprint.Status = work.SprintStatusActive
		case "future":
			sprint.Status = work.SprintStatusFuture
		default:
			sprint.Status = work.SprintStatus(4) // unset
		}
		date.ConvertToModel(r.Attributes.StartDate, &sprint.StartedDate)
		date.ConvertToModel(r.Attributes.FinishDate, &sprint.EndedDate)
		date.ConvertToModel(r.Attributes.FinishDate, &sprint.CompletedDate)
		sprints = append(sprints, &sprint)
	}
	return sprints, err
}
