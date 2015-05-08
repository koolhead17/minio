/*
 * Minimalist Object Storage, (C) 2015 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"os"
	"os/user"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/minio-io/cli"
	"github.com/minio-io/minio/pkg/featureflags"
	"github.com/minio-io/minio/pkg/iodine"
	"github.com/minio-io/minio/pkg/server"
	"github.com/minio-io/minio/pkg/server/httpserver"
)

var globalDebugFlag = false

var commands = []cli.Command{
	modeCmd,
}

var modeCommands = []cli.Command{
	memoryCmd,
	donutCmd,
}

var modeCmd = cli.Command{
	Name:        "mode",
	Subcommands: modeCommands,
	Description: "Mode of execution",
}

var memoryCmd = cli.Command{
	Name:        "memory",
	Description: "Limit maximum memory usage to SIZE in [B, KB, MB, GB]",
	Action:      runMemory,
	CustomHelpTemplate: `NAME:
  minio mode {{.Name}} - {{.Description}}

USAGE:
  minio mode {{.Name}} limit SIZE expire TIME

EXAMPLES:
  1. Limit maximum memory usage to 64MB with 1 hour expiration
      $ minio mode {{.Name}} limit 64MB expire 1h

  2. Limit maximum memory usage to 4GB with no expiration
      $ minio mode {{.Name}} limit 4GB
`,
}

var donutCmd = cli.Command{
	Name:        "donut",
	Description: "Specify a path to instantiate donut",
	Action:      runDonut,
	CustomHelpTemplate: `NAME:
  minio mode {{.Name}} - {{.Description}}

USAGE:
  minio mode {{.Name}} PATH

EXAMPLES:
  1. Create a donut volume under "/mnt/backup"
      $ minio mode {{.Name}} /mnt/backup

  2. Create a temporary donut volume under "/tmp"
      $ minio mode {{.Name}} /tmp

  3. Create a donut volume under collection of paths
      $ minio mode {{.Name}} /mnt/backup2014feb /mnt/backup2014feb

`,
}

var flags = []cli.Flag{
	cli.StringFlag{
		Name:  "api-address",
		Value: ":9000",
		Usage: "address for incoming API requests",
	},
	cli.StringFlag{
		Name:  "web-address",
		Value: ":9001",
		Usage: "address for incoming Management UI requests",
	},
	cli.StringFlag{
		Name:  "cert",
		Hide:  true,
		Value: "",
		Usage: "cert.pem",
	},
	cli.StringFlag{
		Name:  "key",
		Hide:  true,
		Value: "",
		Usage: "key.pem",
	},
	cli.BoolFlag{
		Name:  "debug",
		Usage: "print debug information",
	},
}

func init() {
	// Check for the environment early on and gracefuly report.
	_, err := user.Current()
	if err != nil {
		Fatalf("Unable to obtain user's home directory. \nError: %s\n", err)
	}
}

func runMemory(c *cli.Context) {
	if len(c.Args()) == 0 || len(c.Args())%2 != 0 {
		cli.ShowCommandHelpAndExit(c, "memory", 1) // last argument is exit code
	}
	apiServerConfig := getAPIServerConfig(c)

	var maxMemory uint64
	maxMemorySet := false

	var expiration time.Duration
	expirationSet := false

	var err error

	args := c.Args()
	for len(args) > 0 {
		switch args.First() {
		case "limit":
			{
				if maxMemorySet {
					Fatalln("Limit should be set only once")
				}
				args = args.Tail()
				maxMemory, err = humanize.ParseBytes(args.First())
				if err != nil {
					Fatalf("Invalid memory size [%s] passed. Reason: %s\n", args.First(), iodine.New(err, nil))
				}
				if maxMemory < 1024*1024*10 {
					Fatalf("Invalid memory size [%s] passed. Should be greater than 10M\n", args.First())
				}
				args = args.Tail()
				maxMemorySet = true
			}
		case "expire":
			{
				if expirationSet {
					Fatalln("Expiration should be set only once")
				}
				args = args.Tail()
				expiration, err = time.ParseDuration(args.First())
				if err != nil {
					Fatalf("Invalid expiration time [%s] passed. Reason: %s\n", args.First(), iodine.New(err, nil))
				}
				args = args.Tail()
				expirationSet = true
			}
		default:
			{
				cli.ShowCommandHelpAndExit(c, "memory", 1) // last argument is exit code
			}
		}
	}
	if maxMemorySet == false {
		Fatalln("Memory limit must be set")
	}
	memoryDriver := server.MemoryFactory{
		Config:     apiServerConfig,
		MaxMemory:  maxMemory,
		Expiration: expiration,
	}
	apiServer := memoryDriver.GetStartServerFunc()
	webServer := getWebServerConfigFunc(c)
	servers := []server.StartServerFunc{apiServer, webServer}
	server.StartMinio(servers)
}

func runDonut(c *cli.Context) {
	u, err := user.Current()
	if err != nil {
		Fatalf("Unable to determine current user. Reason: %s\n", err)
	}
	if len(c.Args()) < 1 {
		cli.ShowCommandHelpAndExit(c, "donut", 1) // last argument is exit code
	}

	// supporting multiple paths
	var paths []string
	if strings.TrimSpace(c.Args().First()) == "" {
		p := path.Join(u.HomeDir, "minio-storage", "donut")
		paths = append(paths, p)
	} else {
		for _, arg := range c.Args() {
			paths = append(paths, strings.TrimSpace(arg))
		}
	}
	apiServerConfig := getAPIServerConfig(c)
	donutDriver := server.DonutFactory{
		Config: apiServerConfig,
		Paths:  paths,
	}
	apiServer := donutDriver.GetStartServerFunc()
	webServer := getWebServerConfigFunc(c)
	servers := []server.StartServerFunc{apiServer, webServer}
	server.StartMinio(servers)
}

func getAPIServerConfig(c *cli.Context) httpserver.Config {
	certFile := c.String("cert")
	keyFile := c.String("key")
	if (certFile != "" && keyFile == "") || (certFile == "" && keyFile != "") {
		Fatalln("Both certificate and key are required to enable https.")
	}
	tls := (certFile != "" && keyFile != "")
	return httpserver.Config{
		Address:  c.GlobalString("api-address"),
		TLS:      tls,
		CertFile: certFile,
		KeyFile:  keyFile,
	}
}

func getWebServerConfigFunc(c *cli.Context) server.StartServerFunc {
	config := httpserver.Config{
		Address:  c.GlobalString("web-address"),
		TLS:      false,
		CertFile: "",
		KeyFile:  "",
	}
	webDrivers := server.WebFactory{
		Config: config,
	}
	return webDrivers.GetStartServerFunc()
}

// Build date
var BuildDate string

// getBuildDate -
func getBuildDate() string {
	if BuildDate == "" {
		return ""
	}
	t, _ := time.Parse(time.RFC3339Nano, BuildDate)
	return t.String()
}

// Tries to get os/arch/platform specific information
// Returns a map of current os/arch/platform/memstats
func getSystemData() map[string]string {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	memstats := &runtime.MemStats{}
	runtime.ReadMemStats(memstats)
	mem := fmt.Sprintf("Used: %s | Allocated: %s | Used-Heap: %s | Allocated-Heap: %s",
		humanize.Bytes(memstats.Alloc),
		humanize.Bytes(memstats.TotalAlloc),
		humanize.Bytes(memstats.HeapAlloc),
		humanize.Bytes(memstats.HeapSys))
	platform := fmt.Sprintf("Host: %s | OS: %s | Arch: %s",
		host,
		runtime.GOOS,
		runtime.GOARCH)
	goruntime := fmt.Sprintf("Version: %s | CPUs: %s", runtime.Version(), strconv.Itoa(runtime.NumCPU()))
	return map[string]string{
		"PLATFORM": platform,
		"RUNTIME":  goruntime,
		"MEM":      mem,
	}
}

// Version is based on MD5SUM of its binary
var Version = mustHashBinarySelf()

func main() {
	// enable features
	featureflags.Enable(featureflags.MultipartPutObject)

	// set up iodine
	iodine.SetGlobalState("minio.version", Version)
	iodine.SetGlobalState("minio.starttime", time.Now().Format(time.RFC3339))

	// set up go max processes
	runtime.GOMAXPROCS(runtime.NumCPU())

	// set up app
	app := cli.NewApp()
	app.Name = "minio"
	app.Version = Version
	app.Compiled = getBuildDate()
	app.Author = "Minio.io"
	app.Usage = "Minimalist Object Storage"
	app.Flags = flags
	app.Commands = commands
	app.Before = func(c *cli.Context) error {
		if c.GlobalBool("debug") {
			app.ExtraInfo = getSystemData()
		}
		return nil
	}
	app.CustomAppHelpTemplate = `NAME:
  {{.Name}} - {{.Usage}}

USAGE:
  {{.Name}} {{if .Flags}}[global flags] {{end}}command{{if .Flags}} [command flags]{{end}} [arguments...]

COMMANDS:
  {{range .Commands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
  {{end}}{{if .Flags}}
GLOBAL FLAGS:
  {{range .Flags}}{{.}}
  {{end}}{{end}}
VERSION:
  {{.Version}}
  {{if .Compiled}}
BUILD:
  {{.Compiled}}{{end}}
  {{range $key, $value := .ExtraInfo}}
{{$key}}:
  {{$value}}
{{end}}
`

	app.RunAndExitOnError()
}