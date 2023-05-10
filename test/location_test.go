package test

import (
	"github.com/freegle/iznik-server-go/location"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestClosest(t *testing.T) {
	id, name, areaname := location.ClosestPostcode(55.957571, -3.205333)
	assert.Greater(t, id, uint64(0))
	assert.Greater(t, len(name), 0)
	assert.Greater(t, len(areaname), 0)

	location := location.FetchSingle(id)
	assert.Equal(t, name, location.Name)
	assert.Equal(t, areaname, location.Areaname)
}
