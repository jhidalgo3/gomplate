//+build !windows

package integration

//xxx+build integration
import (
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strconv"

	. "gopkg.in/check.v1"

	"github.com/gotestyourself/gotestyourself/fs"
	"github.com/gotestyourself/gotestyourself/icmd"
)

type VaultDatasourcesSuite struct {
	tmpDir      *fs.Dir
	pidDir      *fs.Dir
	vaultAddr   string
	vaultResult *icmd.Result
	v           *vaultClient
}

var _ = Suite(&VaultDatasourcesSuite{})

const vaultRootToken = "00000000-1111-2222-3333-444455556666"

func (s *VaultDatasourcesSuite) SetUpSuite(c *C) {
	s.startVault(c)

	var err error
	s.v, err = createVaultClient(s.vaultAddr, vaultRootToken)
	if err != nil {
		c.Fatal(err)
	}

	err = s.v.vc.Sys().PutPolicy("writepol", `path "*" {
  policy = "write"
}`)
	if err != nil {
		c.Fatal(err)
	}
	err = s.v.vc.Sys().PutPolicy("readpol", `path "*" {
  policy = "read"
}`)
	if err != nil {
		c.Fatal(err)
	}
}

func (s *VaultDatasourcesSuite) startVault(c *C) {
	s.pidDir = fs.NewDir(c, "gomplate-inttests-vaultpid")
	s.tmpDir = fs.NewDir(c, "gomplate-inttests",
		fs.WithFile("config.json", `{
		"pid_file": "`+s.pidDir.Join("vault.pid")+`"
		}`),
	)

	// rename any existing token so it doesn't get overridden
	u, _ := user.Current()
	homeDir := u.HomeDir
	tokenFile := path.Join(homeDir, ".vault-token")
	info, err := os.Stat(tokenFile)
	if err == nil && info.Mode().IsRegular() {
		os.Rename(tokenFile, path.Join(homeDir, ".vault-token.bak"))
	}

	_, s.vaultAddr = freeport()
	vault := icmd.Command("vault", "server",
		"-dev",
		"-dev-root-token-id="+vaultRootToken,
		"-log-level=err",
		"-dev-listen-address="+s.vaultAddr,
		"-config="+s.tmpDir.Join("config.json"),
	)
	s.vaultResult = icmd.StartCmd(vault)

	c.Logf("Fired up Vault: %v", vault)

	err = waitForURL(c, "http://"+s.vaultAddr+"/v1/sys/health")
	if err != nil {
		c.Fatal(err)
	}
}

func (s *VaultDatasourcesSuite) TearDownSuite(c *C) {
	defer s.tmpDir.Remove()
	defer s.pidDir.Remove()

	p, err := ioutil.ReadFile(s.pidDir.Join("vault.pid"))
	if err != nil {
		c.Fatal(err)
	}
	pid, err := strconv.Atoi(string(p))
	if err != nil {
		c.Fatal(err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		c.Fatal(err)
	}
	err = process.Kill()
	if err != nil {
		c.Fatal(err)
	}

	// restore old token if it was backed up
	u, _ := user.Current()
	homeDir := u.HomeDir
	tokenFile := path.Join(homeDir, ".vault-token.bak")
	info, err := os.Stat(tokenFile)
	if err == nil && info.Mode().IsRegular() {
		os.Rename(tokenFile, path.Join(homeDir, ".vault-token"))
	}
}

func (s *VaultDatasourcesSuite) TestTokenAuth(c *C) {
	s.v.vc.Logical().Write("secret/foo", map[string]interface{}{"value": "bar"})
	defer s.v.vc.Logical().Delete("secret/foo")
	tok, err := s.v.tokenCreate("readpol", 4)
	if err != nil {
		c.Fatal(err)
	}

	result := icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_TOKEN=" + tok,
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})

	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault+http://"+s.v.addr+"/secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_TOKEN=" + tok,
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})

	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "bar").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_TOKEN=" + tok,
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 1, Err: "No value found for [bar] from datasource 'vault'"})

	tokFile := fs.NewFile(c, "test-vault-token", fs.WithContent(tok))
	defer tokFile.Remove()
	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_TOKEN_FILE=" + tokFile.Path(),
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})
}

func (s *VaultDatasourcesSuite) TestUserPassAuth(c *C) {
	s.v.vc.Logical().Write("secret/foo", map[string]interface{}{"value": "bar"})
	defer s.v.vc.Logical().Delete("secret/foo")
	err := s.v.vc.Sys().EnableAuth("userpass", "userpass", "")
	if err != nil {
		c.Fatal(err)
	}
	err = s.v.vc.Sys().EnableAuth("userpass2", "userpass", "")
	if err != nil {
		c.Fatal(err)
	}
	defer s.v.vc.Sys().DisableAuth("userpass")
	defer s.v.vc.Sys().DisableAuth("userpass2")
	_, err = s.v.vc.Logical().Write("auth/userpass/users/dave", map[string]interface{}{
		"password": "foo", "ttl": "10s", "policies": "readpol"})
	if err != nil {
		c.Fatal(err)
	}
	_, err = s.v.vc.Logical().Write("auth/userpass2/users/dave", map[string]interface{}{
		"password": "bar", "ttl": "10s", "policies": "readpol"})
	if err != nil {
		c.Fatal(err)
	}

	result := icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_AUTH_USERNAME=dave", "VAULT_AUTH_PASSWORD=foo",
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})

	userFile := fs.NewFile(c, "test-vault-user", fs.WithContent("dave"))
	passFile := fs.NewFile(c, "test-vault-pass", fs.WithContent("foo"))
	defer userFile.Remove()
	defer passFile.Remove()
	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_AUTH_USERNAME_FILE=" + userFile.Path(),
			"VAULT_AUTH_PASSWORD_FILE=" + passFile.Path(),
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})

	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_AUTH_USERNAME=dave", "VAULT_AUTH_PASSWORD=bar",
			"VAULT_AUTH_USERPASS_MOUNT=userpass2",
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})
}

func (s *VaultDatasourcesSuite) TestAppRoleAuth(c *C) {
	s.v.vc.Logical().Write("secret/foo", map[string]interface{}{"value": "bar"})
	defer s.v.vc.Logical().Delete("secret/foo")
	err := s.v.vc.Sys().EnableAuth("approle", "approle", "")
	if err != nil {
		c.Fatal(err)
	}
	err = s.v.vc.Sys().EnableAuth("approle2", "approle", "")
	if err != nil {
		c.Fatal(err)
	}
	defer s.v.vc.Sys().DisableAuth("approle")
	defer s.v.vc.Sys().DisableAuth("approle2")
	_, err = s.v.vc.Logical().Write("auth/approle/role/testrole", map[string]interface{}{
		"secret_id_ttl": "10s", "token_ttl": "20s",
		"secret_id_num_uses": "1", "policies": "readpol",
	})
	if err != nil {
		c.Fatal(err)
	}
	_, err = s.v.vc.Logical().Write("auth/approle2/role/testrole", map[string]interface{}{
		"secret_id_ttl": "10s", "token_ttl": "20s",
		"secret_id_num_uses": "1", "policies": "readpol",
	})
	if err != nil {
		c.Fatal(err)
	}

	rid, _ := s.v.vc.Logical().Read("auth/approle/role/testrole/role-id")
	roleID := rid.Data["role_id"].(string)
	sid, _ := s.v.vc.Logical().Write("auth/approle/role/testrole/secret-id", nil)
	secretID := sid.Data["secret_id"].(string)
	result := icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_ROLE_ID=" + roleID,
			"VAULT_SECRET_ID=" + secretID,
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})

	rid, _ = s.v.vc.Logical().Read("auth/approle2/role/testrole/role-id")
	roleID = rid.Data["role_id"].(string)
	sid, _ = s.v.vc.Logical().Write("auth/approle2/role/testrole/secret-id", nil)
	secretID = sid.Data["secret_id"].(string)
	result = icmd.RunCmd(icmd.Command(GomplateBin,
		"-d", "vault=vault:///secret",
		"-i", `{{(ds "vault" "foo").value}}`,
	), func(c *icmd.Cmd) {
		c.Env = []string{
			"VAULT_ADDR=http://" + s.v.addr,
			"VAULT_ROLE_ID=" + roleID,
			"VAULT_SECRET_ID=" + secretID,
			"VAULT_AUTH_APPROLE_MOUNT=approle2",
		}
	})
	result.Assert(c, icmd.Expected{ExitCode: 0, Out: "bar"})
}
