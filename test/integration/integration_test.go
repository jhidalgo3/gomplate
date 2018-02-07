package integration

import (
	"encoding/json"
	"go/build"
	"log"
	"net/http"
	"testing"

	"github.com/gotestyourself/gotestyourself/icmd"
	. "gopkg.in/check.v1"
)

var (
	GomplateBin = build.Default.GOPATH + "/src/github.com/hairyhenderson/gomplate/bin/gomplate"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

// a convenience...
func inOutTest(c *C, i string, o string) {
	result := icmd.RunCommand(GomplateBin, "-i", i)
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: o})
}

// mirrorHandler - reflects back the HTTP headers from the request
func mirrorHandler(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Headers http.Header `json:"headers"`
	}
	req := Req{r.Header}
	b, err := json.Marshal(req)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
