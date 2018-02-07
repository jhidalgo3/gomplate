//+build !windows

package integration

//xxx+build integration
import (
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

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

const CONSUL_ROOT_TOKEN = "00000000-1111-2222-3333-444455556666"

func (s *ConsulDatasourcesSuite) SetUpSuite(c *C) {
	s.tmpDir = fs.NewDir(c, "gomplate-inttests",
		fs.WithFile(
			"consul.json",
			`{"acl_datacenter": "dc1", "acl_master_token": "`+CONSUL_ROOT_TOKEN+`"}`,
		),
	)
	port, l := freeport()
	s.consulAddr = l.Addr().String()
	consul := icmd.Command("consul", "agent",
		"-dev",
		"-config-file="+s.tmpDir.Join("consul.json"),
		"-log-level=err",
		"-http-port="+strconv.Itoa(port),
		"-pid-file="+s.tmpDir.Join("consul.pid"),
	)
	s.consulResult = icmd.StartCmd(consul)

	// c.Errorf("Fired up Consul: %v", consul)

	err := s.waitForConsul(c)
	if err != nil {
		c.Fatal(err)
	}
}

func (s *ConsulDatasourcesSuite) waitForConsul(c *C) error {
	client := http.DefaultClient
	retries := 10
	for retries > 0 {
		retries--
		resp, err := client.Get("http://" + s.consulAddr + "/v1/status/leader")
		if err != nil {
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		c.Fatal(body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func freeport() (port int, l *net.TCPListener) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv6loopback})
	defer l.Close()
	if err != nil {
		panic(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	port = addr.Port
	return port, l
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

func (s *ConsulDatasourcesSuite) TestConsulDatasource(c *C) {
	result := icmd.RunCommand(GomplateBin,
		"-d", "consul=consul://",
		"-i", `{{(ds "consul" "foo")}}`,
	)
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})
}
