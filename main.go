// Copyright (C) 2018-present Juicedata Inc.

package main

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/juicedata/juicefs/pkg/object"
	"github.com/juicedata/juicefs/pkg/sync"
	"github.com/juicedata/juicefs/pkg/utils"
	"github.com/juicedata/juicesync/versioninfo"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

var logger = utils.GetLogger("juicesync")

func supportHTTPS(name, endpoint string) bool {
	switch name {
	case "ufile":
		return !(strings.Contains(endpoint, ".internal-") || strings.HasSuffix(endpoint, ".ucloud.cn"))
	case "oss":
		return !(strings.Contains(endpoint, ".vpc100-oss") || strings.Contains(endpoint, "internal.aliyuncs.com"))
	case "jss":
		return false
	case "s3":
		ps := strings.SplitN(strings.Split(endpoint, ":")[0], ".", 2)
		if len(ps) > 1 && net.ParseIP(ps[1]) != nil {
			return false
		}
	case "minio":
		return false
	}
	return true
}

// Check if uri is local file path
func isFilePath(uri string) bool {
	// check drive pattern when running on Windows
	if runtime.GOOS == "windows" &&
		len(uri) > 1 && (('a' <= uri[0] && uri[0] <= 'z') ||
		('A' <= uri[0] && uri[0] <= 'Z')) && uri[1] == ':' {
		return true
	}
	return !strings.Contains(uri, ":")
}


func createStorage(uri string, conf *sync.Config) (object.ObjectStorage, error) {
	if !strings.Contains(uri, "://") {
		if isFilePath(uri) {
			absPath, err := filepath.Abs(uri)
			if err != nil {
				logger.Fatalf("invalid path: %s", err.Error())
			}
			if !strings.HasPrefix(absPath, "/") { // Windows path
				absPath = "/" + strings.Replace(absPath, "\\", "/", -1)
			}
			if strings.HasSuffix(uri, "/") {
				absPath += "/"
			}

			// Windows: file:///C:/a/b/c, Unix: file:///a/b/c
			uri = "file://" + absPath
		} else { // sftp
			var user string
			if strings.Contains(uri, "@") {
				parts := strings.Split(uri, "@")
				user = parts[0]
				uri = parts[1]
			}
			var pass string
			if strings.Contains(user, ":") {
				parts := strings.Split(user, ":")
				user = parts[0]
				pass = parts[1]
			} else if os.Getenv("SSH_PRIVATE_KEY_PATH") == "" {
				fmt.Print("Enter Password: ")
				bytePassword, err := term.ReadPassword(int(syscall.Stdin))
				if err != nil {
					logger.Fatalf("Read password: %s", err.Error())
				}
				pass = string(bytePassword)
			}
			return object.CreateStorage("sftp", uri, user, pass)
		}
	}
	u, err := url.Parse(uri)
	if err != nil {
		logger.Fatalf("Can't parse %s: %s", uri, err.Error())
	}
	user := u.User
	var accessKey, secretKey string
	if user != nil {
		accessKey = user.Username()
		secretKey, _ = user.Password()
	}
	name := strings.ToLower(u.Scheme)
	endpoint := u.Host
	if name == "file" {
		endpoint = u.Path
	} else if name == "hdfs" {
	} else if !conf.NoHTTPS && supportHTTPS(name, endpoint) {
		endpoint = "https://" + endpoint
	} else {
		endpoint = "http://" + endpoint
	}

	store, err := object.CreateStorage(name, endpoint, accessKey, secretKey)
	if err != nil {
		return nil, fmt.Errorf("create %s %s: %s", name, endpoint, err)
	}
	if conf.Perms {
		if _, ok := store.(object.FileSystem); !ok {
			logger.Warnf("%s is not a file system, can not preserve permissions", store)
			conf.Perms = false
		}
	}
	if name != "file" && len(u.Path) > 1 {
		store = object.WithPrefix(store, u.Path[1:])
	}
	return store, nil
}

func run(c *cli.Context) error {
	config := sync.NewConfigFromCli(c)
	go func() { _ = http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", config.HTTPPort), nil) }()

	if config.Verbose {
		utils.SetLogLevel(logrus.DebugLevel)
	} else if config.Quiet {
		utils.SetLogLevel(logrus.ErrorLevel)
	}
	utils.InitLoggers(false)

	// Windows support `\` and `/` as its separator, Unix only use `/`
	srcURL := strings.Replace(c.Args().Get(0), "\\", "/", -1)
	dstURL := strings.Replace(c.Args().Get(1), "\\", "/", -1)
	if strings.HasSuffix(srcURL, "/") != strings.HasSuffix(dstURL, "/") {
		logger.Fatalf("SRC and DST should both end with path separator or not!")
	}
	src, err := createStorage(srcURL, config)
	if err != nil {
		return err
	}
	dst, err := createStorage(dstURL, config)
	if err != nil {
		return err
	}
	return sync.Sync(src, dst, config)
}

func main() {
	cli.VersionFlag = &cli.BoolFlag{
		Name: "version", Aliases: []string{"V"},
		Usage: "print only the version",
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Println(versioninfo.Version())
	}

	app := cli.NewApp()
	app.Name = versioninfo.NAME
	app.Usage = "rsync for cloud storage"
	app.UsageText = versioninfo.USAGE
	app.Version = versioninfo.VERSION
	app.Copyright = "AGPLv3"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "start",
			Aliases: []string{"s"},
			Value:   "",
			Usage:   "the first `KEY` to sync",
		},
		&cli.StringFlag{
			Name:    "end",
			Aliases: []string{"e"},
			Value:   "",
			Usage:   "the last `KEY` to sync",
		},
		&cli.IntFlag{
			Name:    "threads",
			Aliases: []string{"p"},
			Value:   10,
			Usage:   "number of concurrent threads",
		},
		&cli.IntFlag{
			Name:  "http-port",
			Value: 6070,
			Usage: "HTTP `PORT` to listen to",
		},
		&cli.BoolFlag{
			Name:    "update",
			Aliases: []string{"u"},
			Usage:   "update existing file if the source is newer",
		},
		&cli.BoolFlag{
			Name:    "force-update",
			Aliases: []string{"f"},
			Usage:   "always update existing file",
		},
		&cli.BoolFlag{
			Name:  "perms",
			Usage: "preserve permissions",
		},
		&cli.BoolFlag{
			Name:  "dirs",
			Usage: "Sync directories or holders",
		},
		&cli.BoolFlag{
			Name:  "dry",
			Usage: "don't copy file",
		},
		&cli.BoolFlag{
			Name:    "delete-src",
			Aliases: []string{"deleteSrc"},
			Usage:   "delete objects from source after synced",
		},
		&cli.BoolFlag{
			Name:    "delete-dst",
			Aliases: []string{"deleteDst"},
			Usage:   "delete extraneous objects from destination",
		},
		&cli.StringSliceFlag{
			Name:  "exclude",
			Usage: "exclude keys containing `PATTERN` (POSIX regular expressions)",
		},
		&cli.StringSliceFlag{
			Name:  "include",
			Usage: "only include keys containing `PATTERN` (POSIX regular expressions)",
		},
		&cli.StringFlag{
			Name:  "manager",
			Usage: "manager address",
		},
		&cli.StringSliceFlag{
			Name:  "worker",
			Usage: "hosts (seperated by comma) to launch worker",
		},
		&cli.IntFlag{
			Name:  "bwlimit",
			Usage: "limit bandwidth in Mbps (default: unlimited)",
		},
		&cli.BoolFlag{
			Name:  "no-https",
			Usage: "donot use HTTPS",
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "turn on debug log",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "change log level to ERROR",
		},
	}
	app.Action = func(c *cli.Context) error {
		if c.Args().Len() != 2 {
			logger.Errorf(versioninfo.USAGE)
			return nil
		}
		return run(c)
	}

	err := app.Run(os.Args)
	if err != nil {
		logger.Fatalf("Error running juicesync: %s", err)
	}
}
