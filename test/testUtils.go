package test

import (
	"io"
	"net/http"
	"strings"
)

func rsp(response *http.Response) []byte {
	buf := new(strings.Builder)
	io.Copy(buf, response.Body)
	//fmt.Println(buf.String())
	return []byte(buf.String())
}
