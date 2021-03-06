package codetainer

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"

	version "github.com/hashicorp/go-version"

	"github.com/BurntSushi/toml"
	docker "github.com/fsouza/go-dockerclient"
)

var globalConfigPath string = "/etc/codetainer/config.toml"
var globalDbPath string = "/var/codetainer/codetainer.db"

var (
	DefaultConfigFileSettings = `# Docker API server and port 
DockerServer = "localhost"
DockerPort = 4500`
	GlobalConfig Config
)

//
// detectConfigPath will return the path to the configuration file.
// Use either a global path: /etc/codetainer/config.toml
// Or a local path ~/.codetainer/config.toml
//
func detectConfigPath() (string, error) {

	if fileExists(globalConfigPath) {
		return globalConfigPath, nil
	}
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	basePath := path.Join(usr.HomeDir, ".codetainer")
	if _, err := os.Stat(basePath); err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(basePath, 0700)
			if err != nil {
				return "", err
			}
		}
	}
	return path.Join(basePath, "config.toml"), nil
}

// detectDataabsePath will return the path to the database file.
// Use either a global path: /etc/codetainer/codetainer.db
// Or a local path ~/.codetainer/codetainer.db

func detectDatabasePath() (string, error) {

	if fileExists(globalDbPath) {
		return globalDbPath, nil
	}
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	basePath := path.Join(usr.HomeDir, ".codetainer")
	if _, err := os.Stat(basePath); err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(basePath, 0700)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}
	return path.Join(basePath, "codetainer.db"), nil
}

type Config struct {
	DockerServerUseHttps    bool
	DockerServer            string
	DockerPort              int
	DatabasePath            string
	database                *Database
	currentDockerApiVersion string
}

func (c *Config) Url() string {
	// TODO: make this configurable
	return "http://localhost:3000"
}

func (c *Config) GetDatabase() (*Database, error) {
	// TODO cache db

	if c.database != nil {
		return c.database, nil
	}

	db, err := NewDatabase(c.GetDatabasePath())
	if err != nil {
		return nil, err
	}
	c.database = db
	return c.database, nil
}

func (c *Config) GetDatabasePath() string {

	if c.DatabasePath == "" {
		p, err := detectDatabasePath()
		c.DatabasePath = p
		if err != nil {
			Log.Fatal("Unable to create database at ~/.codetainer/codetainer.db or "+globalDbPath, err)
		}
	}
	Log.Debugf("Using database path: %s", c.DatabasePath)
	return c.DatabasePath
}

func (c *Config) UtilsPath() string {
	dir, _ := filepath.Abs(filepath.Dir(os.Args[0]))
	return path.Join(dir, "util")
}

func (c *Config) GetDockerClient() (*docker.Client, error) {
	endpoint := c.GetDockerEndpoint()
	return docker.NewClient(endpoint)
}

func (c *Config) testDockerClient() error {
	endpoint, err := c.GetDockerClient()
	if err != nil {
		return err
	}
	return endpoint.Ping()
}

func (c *Config) testDockerVersion() error {
	endpoint, err := c.GetDockerClient()
	if err != nil {
		return err
	}
	ev, err := endpoint.Version()
	if err != nil {

		return err
	}
	currVersion := ev.Get("ApiVersion")
	activeVersion, err := version.NewVersion(currVersion)
	supportedVersion, err := version.NewVersion(DockerApiVersion)
	if activeVersion.LessThan(supportedVersion) {
		return errors.New(currVersion + " version is lower than supported Docker version of " + DockerApiVersion + ". You will need to upgrade docker.")
	}

	Log.Debug("Found docker API version: ", currVersion)
	c.currentDockerApiVersion = currVersion
	return nil
}

func (c *Config) GetDockerEndpoint() string {
	if c.DockerServerUseHttps {
		return fmt.Sprintf("https://%s:%d", c.DockerServer, c.DockerPort)
	} else {
		return fmt.Sprintf("http://%s:%d", c.DockerServer, c.DockerPort)
	}
}

//
// Ensure a configuration is valid and all dependencies are installed.
//
func (c *Config) TestConfig() bool {
	err := c.testDockerClient()
	if err != nil {
		Log.Fatal(`Unable to connect to Docker API.  Are you sure you have
configured the Docker API to accept remote HTTP connections?

E.g., your docker service needs to have the following parameters in the
command line in order to use web sockets:

  /usr/bin/docker -d -H tcp://127.0.0.1:4500

Please also check your config.toml has the correct configuration for the DockerServer
and DockerPort:

  # Docker API server and port
  DockerServer = "localhost"
  DockerPort = 4500
`)

	}
	err = c.testDockerVersion()
	if err != nil {
		Log.Fatal(err)
	}

	return true
}

func NewConfig(configPath string) (*Config, error) {
	var err error
	if configPath == "" {
		configPath, err = detectConfigPath()
		if err != nil {
			Log.Fatal("Unable to load config from ~/.codetainer/config.toml or /etc/codetainer/config.toml", err)
		}
	}

	Log.Debugf("Loading %s configurations from %s", Name, configPath)
	config := &Config{}

	if !IsExist(configPath) {

		configData := []byte(DefaultConfigFileSettings)

		f, err := os.Create(configPath)

		if err != nil {
			Log.Error(err)
			Log.Fatalf("Unable to create configuration file: %s.", configPath)
		}

		_, err = f.Write(configData)

		if err != nil {
			Log.Error(err)
			Log.Fatalf("Unable to create configuration file: %s.", configPath)
		}

		f.Sync()
		f.Close()
	}

	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return config, err
	}
	return config, nil
}
