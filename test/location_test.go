package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/location"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestClosest(t *testing.T) {
	l := location.ClosestPostcode(55.957571, -3.205333)
	id := l.ID
	assert.NotNil(t, id)
	name := l.Name
	areaname := l.Areaname
	assert.Greater(t, id, uint64(0))
	assert.Greater(t, len(name), 0)
	assert.Greater(t, len(areaname), 0)

	location := location.FetchSingle(id)
	assert.Equal(t, name, location.Name)
	assert.Equal(t, areaname, location.Areaname)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/"+fmt.Sprint(id), nil))
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &location)
	assert.Equal(t, location.ID, id)
}

func TestTypeahead(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead?q=EH3&groupsnear=true&limit=1000", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var locations []location.Location
	json2.Unmarshal(rsp(resp), &locations)
	assert.Greater(t, len(locations), 0)
	assert.Greater(t, len(locations[0].Name), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead?p=EH3", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestLatLng(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/latlng?lat=55.957571&lng=-3.205333", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var location location.Location
	json2.Unmarshal(rsp(resp), &location)
	assert.Equal(t, location.Name, "EH3 6SS")
}

func TestLocation_InvalidID(t *testing.T) {
	// Non-integer location ID
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/notanint", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestLocation_NonExistentID(t *testing.T) {
	// Location ID that doesn't exist - handler returns 200 with empty location
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/999999999", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var loc location.Location
	json2.Unmarshal(rsp(resp), &loc)
	assert.Equal(t, uint64(0), loc.ID)
}

func TestTypeahead_MissingQuery(t *testing.T) {
	// No query param at all
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead", nil))
	assert.Equal(t, 404, resp.StatusCode)
}

func TestLatLngGroupsNearOntn(t *testing.T) {
	// LatLng should return groupsnear with the ontn field
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/latlng?lat=55.957571&lng=-3.205333", nil))
	assert.Equal(t, 200, resp.StatusCode)

	// Parse raw JSON to check ontn field exists
	var raw map[string]json2.RawMessage
	json2.Unmarshal(rsp(resp), &raw)
	assert.Contains(t, string(raw["groupsnear"]), "ontn", "groupsnear should include ontn field")
}

func TestTypeaheadAreaField(t *testing.T) {
	// Typeahead response should include area with lat/lng for postcodes that have an areaid
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/typeahead?q=EH3+6&limit=1", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var locations []location.Location
	json2.Unmarshal(rsp(resp), &locations)
	assert.Greater(t, len(locations), 0)

	if len(locations) > 0 && locations[0].Areaid > 0 {
		assert.NotNil(t, locations[0].Area, "Location with areaid should have area field populated")
		assert.Equal(t, locations[0].Areaid, locations[0].Area.ID)
		assert.NotZero(t, locations[0].Area.Lat, "Area should have lat")
		assert.NotZero(t, locations[0].Area.Lng, "Area should have lng")
	}
}

func TestTypeahead_V2Path(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/apiv2/location/typeahead?q=EH3&limit=5", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var locations []location.Location
	json2.Unmarshal(rsp(resp), &locations)
	assert.Greater(t, len(locations), 0)
}

func TestAddresses(t *testing.T) {
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/1687412/addresses", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var addresses []location.Address
	json2.Unmarshal(rsp(resp), &addresses)
	assert.Greater(t, len(addresses), 0)
}
