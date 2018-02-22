//+build !windows

package integration

//xxx+build integration
import (
	"io/ioutil"
	"os"
	"strconv"

	. "gopkg.in/check.v1"

	"github.com/gotestyourself/gotestyourself/fs"
	"github.com/gotestyourself/gotestyourself/icmd"
)

type ConsulDatasourcesSuite struct {
	tmpDir       *fs.Dir
	consulAddr   string
	consulResult *icmd.Result
}

var _ = Suite(&ConsulDatasourcesSuite{})

const consulRootToken = "00000000-1111-2222-3333-444455556666"

func (s *ConsulDatasourcesSuite) SetUpSuite(c *C) {
	s.tmpDir = fs.NewDir(c, "gomplate-inttests",
		fs.WithFile(
			"consul.json",
			`{"acl_datacenter": "dc1", "acl_master_token": "`+consulRootToken+`"}`,
		),
	)
	var port int
	port, s.consulAddr = freeport()
	consul := icmd.Command("consul", "agent",
		"-dev",
		"-config-file="+s.tmpDir.Join("consul.json"),
		"-log-level=err",
		"-http-port="+strconv.Itoa(port),
		"-pid-file="+s.tmpDir.Join("consul.pid"),
	)
	s.consulResult = icmd.StartCmd(consul)

	c.Logf("Fired up Consul: %v", consul)

	err := waitForURL(c, "http://"+s.consulAddr+"/v1/status/leader")
	if err != nil {
		c.Fatal(err)
	}
}

func (s *ConsulDatasourcesSuite) TearDownSuite(c *C) {
	defer s.tmpDir.Remove()

	p, err := ioutil.ReadFile(s.tmpDir.Join("consul.pid"))
	if err != nil {
		c.Fatal(err)
	}
	pid, err := strconv.Atoi(string(p))
	if err != nil {
		c.Fatal(err)
	}
	consulProcess, err := os.FindProcess(pid)
	if err != nil {
		c.Fatal(err)
	}
	err = consulProcess.Kill()
	if err != nil {
		c.Fatal(err)
	}
}

func (s *ConsulDatasourcesSuite) consulPut(c *C, k string, v string) {
	result := icmd.RunCmd(icmd.Command("consul", "kv", "put", k, v),
		func(c *icmd.Cmd) {
			c.Env = []string{"CONSUL_HTTP_ADDR=http://" + s.consulAddr}
		})
	result.Assert(c, icmd.Success)
}

func (s *ConsulDatasourcesSuite) consulDelete(c *C, k string) {
	result := icmd.RunCmd(icmd.Command("consul", "kv", "delete", k),
		func(c *icmd.Cmd) {
			c.Env = []string{"CONSUL_HTTP_ADDR=http://" + s.consulAddr}
		})
	result.Assert(c, icmd.Success)
}

func (s *ConsulDatasourcesSuite) TestConsulDatasource(c *C) {
	s.consulPut(c, "foo", "bar")
	defer s.consulDelete(c, "foo")
	result := icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "consul=consul://",
		"-i", `{{(ds "consul" "foo")}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{"CONSUL_HTTP_ADDR=http://" + s.consulAddr}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})

	s.consulPut(c, "foo", `{"bar": "baz"}`)
	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "consul=consul://?type=application/json",
		"-i", `{{(ds "consul" "foo").bar}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{"CONSUL_HTTP_ADDR=http://" + s.consulAddr}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "baz"})

	s.consulPut(c, "foo", `bar`)
	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "consul=consul://"+s.consulAddr,
		"-i", `{{(ds "consul" "foo")}}`,
	))
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})

	s.consulPut(c, "foo", `bar`)
	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "consul=consul+http://"+s.consulAddr,
		"-i", `{{(ds "consul" "foo")}}`,
	))
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})
}

func (s *ConsulDatasourcesSuite) TestConsulWithVaultAuth(c *C) {
	c.Skip("no vault yet")
	s.consulPut(c, "foo", "bar")
	defer s.consulDelete(c, "foo")
	result := icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "consul=consul://",
		"-i", `{{(ds "consul" "foo")}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"CONSUL_VAULT_ROLE=readonly",
			"CONSUL_HTTP_ADDR=http://" + s.consulAddr,
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})
}
