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
	fmt.Println("TestClosest")
	l := location.ClosestPostcode(55.957571, -3.205333)
	id := l.ID
	name := l.Name
	areaname := l.Areaname
	assert.Greater(t, id, uint64(0))
	assert.Greater(t, len(name), 0)
	assert.Greater(t, len(areaname), 0)

	fmt.Println("Fetch single")
	location := location.FetchSingle(id)
	assert.Equal(t, name, location.Name)
	assert.Equal(t, areaname, location.Areaname)

	fmt.Println("Fetch single via API")
	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/location/"+fmt.Sprint(id), nil))
	fmt.Println("Response", resp)
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
