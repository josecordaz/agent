package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDoubleSlashRegex(t *testing.T) {

	str1 := `[SomeCollection]\\Business Owners`
	test1 := doubleSlashRegex.ReplaceAllString(str1, "")
	assert.Equal(t, test1, "Business Owners")

	str2 := `AwesomeTeam\\Pedro Enrique`
	test2 := doubleSlashRegex.ReplaceAllString(str2, "")
	assert.Equal(t, test2, "Pedro Enrique")

	str3 := `Pedro Enrique`
	test3 := doubleSlashRegex.ReplaceAllString(str3, "")
	assert.Equal(t, test3, "Pedro Enrique")

	str4 := ``
	test4 := doubleSlashRegex.ReplaceAllString(str4, "")
	assert.Equal(t, test4, "")
}
