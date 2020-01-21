package jiracommon

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/pinpt/agent/integrations/pkg/jiracommonapi"
	"github.com/pinpt/integration-sdk/agent"
	"github.com/stretchr/testify/assert"
)

func TestGetExistedStatusesOnly(t *testing.T) {

	assert := assert.New(t)

	allValues := []string{"Selected for Development", "Backlog", "Validated", "Evidence Needed", "Evidence Validated", "Done", "Ready for Promotion", "Work Required", "Rework", "Closed", "To Do", "Awaiting Release", "In Testing", "Control Validation", "In Progress", "Awaiting Validation", "Work Complete", "In Review", "Validate Evidence", "On Hold", "Gathering Evidence"}
	setValues := []string{"Work Complete", "Completed", "Closed", "Done", "Fixed"}

	expected := []string{"Work Complete", "Closed", "Done"}

	actual := getExistedStatusesOnly(allValues, setValues)

	assert.Equal(expected, actual)
}

func TestDefaultStatusStates(t *testing.T) {
	assert := assert.New(t)
	buf, err := ioutil.ReadFile("./testdata/status.json")
	assert.NoError(err)
	var statuses []jiracommonapi.StatusDetail
	assert.NoError(json.Unmarshal(buf, &statuses))
	assert.NotEmpty(statuses)
	var wc agent.WorkStatusResponseWorkConfig
	appendStaticInfo(&wc, statuses)
	assert.Equal([]string{"Work Required", "Rework", "To Do", "Selected for Development", "Awaiting Validation", "Awaiting Release", "Work Complete", "In Testing", "Backlog", "Control Validation", "Evidence Needed", "Validate Evidence", "Acceptance"}, wc.Statuses.OpenStatus)
	assert.Equal([]string{"In Progress", "In Review", "On Hold", "Gathering Evidence"}, wc.Statuses.InProgressStatus)
	assert.Equal([]string{"Closed", "Done", "Ready for Promotion", "Validated", "Evidence Validated"}, wc.Statuses.ClosedStatus)
}
