// Copyright (C) 2018-present Juicedata Inc.

package main

import (
	"fmt"
	"net"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/erikdubbelboer/gspt"
	"github.com/juicedata/juicefs/pkg/object"
	"github.com/juicedata/juicefs/pkg/sync"
	"github.com/juicedata/juicefs/pkg/utils"
	"github.com/juicedata/juicesync/versioninfo"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
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

func extractToken(uri string) (string, string) {
	if submatch := regexp.MustCompile(`^.*:.*:.*(:.*)@.*$`).FindStringSubmatch(uri); len(submatch) == 2 {
		return strings.ReplaceAll(uri, submatch[1], ""), strings.TrimLeft(submatch[1], ":")
	}
	return uri, ""
}

func createSyncStorage(uri string, conf *sync.Config) (object.ObjectStorage, error) {
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
			}
			return object.CreateStorage("sftp", uri, user, pass, "")
		}
	}
	uri, token := extractToken(uri)
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

	var endpoint string
	if name == "file" {
		endpoint = u.Path
	} else if name == "hdfs" {
		endpoint = u.Host
	} else if name == "jfs" {
		endpoint, err = url.PathUnescape(u.Host)
		if err != nil {
			return nil, fmt.Errorf("unescape %s: %s", u.Host, err)
		}
		if os.Getenv(endpoint) != "" {
			conf.Env[endpoint] = os.Getenv(endpoint)
		}
	} else if !conf.NoHTTPS && supportHTTPS(name, u.Host) {
		endpoint = "https://" + u.Host
	} else {
		endpoint = "http://" + u.Host
	}

	isS3PathTypeUrl := isS3PathType(u.Host)
	if name == "minio" || name == "s3" && isS3PathTypeUrl {
		// bucket name is part of path
		endpoint += u.Path
	}

	store, err := object.CreateStorage(name, endpoint, accessKey, secretKey, token)
	if err != nil {
		return nil, fmt.Errorf("create %s %s: %s", name, endpoint, err)
	}

	if conf.Links {
		if _, ok := store.(object.SupportSymlink); !ok {
			logger.Warnf("storage %s does not support symlink, ignore it", uri)
			conf.Links = false
		}
	}

	if conf.Perms {
		if _, ok := store.(object.FileSystem); !ok {
			logger.Warnf("%s is not a file system, can not preserve permissions", store)
			conf.Perms = false
		}
	}
	switch name {
	case "file":
	case "minio":
		if strings.Count(u.Path, "/") > 1 {
			// skip bucket name
			store = object.WithPrefix(store, strings.SplitN(u.Path[1:], "/", 2)[1])
		}
	case "s3":
		if isS3PathTypeUrl && strings.Count(u.Path, "/") > 1 {
			store = object.WithPrefix(store, strings.SplitN(u.Path[1:], "/", 2)[1])
		} else if len(u.Path) > 1 {
			store = object.WithPrefix(store, u.Path[1:])
		}
	default:
		if len(u.Path) > 1 {
			store = object.WithPrefix(store, u.Path[1:])
		}
	}

	return store, nil
}

func isS3PathType(endpoint string) bool {
	//localhost[:8080] 127.0.0.1[:8080]  s3.ap-southeast-1.amazonaws.com[:8080] s3-ap-southeast-1.amazonaws.com[:8080]
	pattern := `^((localhost)|(s3[.-].*\.amazonaws\.com)|((1\d{2}|2[0-4]\d|25[0-5]|[1-9]\d|[1-9])\.((1\d{2}|2[0-4]\d|25[0-5]|[1-9]\d|\d)\.){2}(1\d{2}|2[0-4]\d|25[0-5]|[1-9]\d|\d)))?(:\d*)?$`
	return regexp.MustCompile(pattern).MatchString(endpoint)
}

func doSync(c *cli.Context) error {
	setup(c, 2)
	if c.IsSet("include") && !c.IsSet("exclude") {
		logger.Warnf("The include option needs to be used with the exclude option, otherwise the result of the current sync may not match your expectations")
	}
	config := sync.NewConfigFromCli(c)

	// Windows support `\` and `/` as its separator, Unix only use `/`
	srcURL := c.Args().Get(0)
	dstURL := c.Args().Get(1)
	removePassword(srcURL)
	removePassword(dstURL)
	if runtime.GOOS == "windows" {
		if !strings.Contains(srcURL, "://") {
			srcURL = strings.Replace(srcURL, "\\", "/", -1)
		}
		if !strings.Contains(dstURL, "://") {
			dstURL = strings.Replace(dstURL, "\\", "/", -1)
		}
	}
	if strings.HasSuffix(srcURL, "/") != strings.HasSuffix(dstURL, "/") {
		logger.Fatalf("SRC and DST should both end with path separator or not!")
	}
	src, err := createSyncStorage(srcURL, config)
	if err != nil {
		return err
	}
	dst, err := createSyncStorage(dstURL, config)
	if err != nil {
		return err
	}
	if config.StorageClass != "" {
		if os, ok := dst.(object.SupportStorageClass); ok {
			os.SetStorageClass(config.StorageClass)
		}
	}
	return sync.Sync(src, dst, config)
}

func isFlag(flags []cli.Flag, option string) (bool, bool) {
	if !strings.HasPrefix(option, "-") {
		return false, false
	}
	// --V or -v work the same
	option = strings.TrimLeft(option, "-")
	for _, flag := range flags {
		_, isBool := flag.(*cli.BoolFlag)
		for _, name := range flag.Names() {
			if option == name || strings.HasPrefix(option, name+"=") {
				return true, !isBool && !strings.Contains(option, "=")
			}
		}
	}
	return false, false
}

func reorderOptions(app *cli.App, args []string) []string {
	var newArgs = []string{args[0]}
	var others []string
	globalFlags := append(app.Flags, cli.VersionFlag)
	for i := 1; i < len(args); i++ {
		option := args[i]
		if ok, hasValue := isFlag(globalFlags, option); ok {
			newArgs = append(newArgs, option)
			if hasValue {
				i++
				newArgs = append(newArgs, args[i])
			}
		} else {
			others = append(others, option)
		}
	}
	// no command
	if len(others) == 0 {
		return newArgs
	}
	cmdName := others[0]
	var cmd *cli.Command
	for _, c := range app.Commands {
		if c.Name == cmdName {
			cmd = c
		}
	}
	if cmd == nil {
		// can't recognize the command, skip it
		return append(newArgs, others...)
	}

	newArgs = append(newArgs, cmdName)
	args, others = others[1:], nil
	// -h is valid for all the commands
	cmdFlags := append(cmd.Flags, cli.HelpFlag)
	for i := 0; i < len(args); i++ {
		option := args[i]
		if ok, hasValue := isFlag(cmdFlags, option); ok {
			newArgs = append(newArgs, option)
			if hasValue && len(args[i+1:]) > 0 {
				i++
				newArgs = append(newArgs, args[i])
			}
		} else {
			if strings.HasPrefix(option, "-") && !utils.StringContains(args, "--generate-bash-completion") {
				logger.Fatalf("unknown option: %s", option)
			}
			others = append(others, option)
		}
	}
	return append(newArgs, others...)
}

// Check number of positional arguments, set logger level
func setup(c *cli.Context, n int) {
	if c.NArg() != n {
		fmt.Printf("ERROR: This command requires %d arguments\n", n)
		fmt.Printf("USAGE: %s\n", versioninfo.USAGE)
		os.Exit(1)
	}

	if c.Bool("verbose") {
		utils.SetLogLevel(logrus.DebugLevel)
	} else if c.Bool("quiet") {
		utils.SetLogLevel(logrus.ErrorLevel)
	}
}

func removePassword(uri string) {
	args := make([]string, len(os.Args))
	copy(args, os.Args)
	uri2 := utils.RemovePassword(uri)
	if uri2 != uri {
		for i, a := range os.Args {
			if a == uri {
				args[i] = uri2
				break
			}
		}
	}
	gspt.SetProcTitle(strings.Join(args, " "))
}

func addCategory(f cli.Flag, cat string) {
	switch ff := f.(type) {
	case *cli.StringFlag:
		ff.Category = cat
	case *cli.BoolFlag:
		ff.Category = cat
	case *cli.IntFlag:
		ff.Category = cat
	case *cli.Int64Flag:
		ff.Category = cat
	case *cli.Uint64Flag:
		ff.Category = cat
	case *cli.Float64Flag:
		ff.Category = cat
	case *cli.StringSliceFlag:
		ff.Category = cat
	default:
		panic(f)
	}
}

func addCategories(cat string, flags []cli.Flag) []cli.Flag {
	for _, f := range flags {
		addCategory(f, cat)
	}
	return flags
}

func main() {
	// we have to call this because gspt removes all arguments
	gspt.SetProcTitle(strings.Join(os.Args, " "))
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
	app.Description = `
	This tool spawns multiple threads to concurrently syncs objects of two data storages.
	SRC and DST should be [NAME://][ACCESS_KEY:SECRET_KEY@]BUCKET[.ENDPOINT][/PREFIX].

	Include/exclude pattern rules:
	The include/exclude rules each specify a pattern that is matched against the names of the files that are going to be transferred.  These patterns can take several forms:

	- if the pattern ends with a / then it will only match a directory, not a file, link, or device.
	- it chooses between doing a simple string match and wildcard matching by checking if the pattern contains one of these three wildcard characters: '*', '?', and '[' .
	- a '*' matches any non-empty path component (it stops at slashes).
	- a '?' matches any character except a slash (/).
	- a '[' introduces a character class, such as [a-z] or [[:alpha:]].
	- in a wildcard pattern, a backslash can be used to escape a wildcard character, but it is matched literally when no wildcards are present.
	- it does a prefix match of pattern, i.e. always recursive

	Examples:
	# Sync object from OSS to S3
	$ juicesync oss://mybucket.oss-cn-shanghai.aliyuncs.com s3://mybucket.s3.us-east-2.amazonaws.com

	# Sync objects from S3 to JuiceFS
	$ juicefs mount -d redis://localhost /mnt/jfs
	$ juicesync s3://mybucket.s3.us-east-2.amazonaws.com/ /mnt/jfs/

	# SRC: a1/b1,a2/b2,aaa/b1   DST: empty   sync result: aaa/b1
	$ juicesync --exclude='a?/b*' s3://mybucket.s3.us-east-2.amazonaws.com/ /mnt/jfs/

	# SRC: a1/b1,a2/b2,aaa/b1   DST: empty   sync result: a1/b1,aaa/b1
	$ juicesync --include='a1/b1' --exclude='a[1-9]/b*' s3://mybucket.s3.us-east-2.amazonaws.com/ /mnt/jfs/

	# SRC: a1/b1,a2/b2,aaa/b1,b1,b2  DST: empty   sync result: a1/b1,b2
	$ juicesync --include='a1/b1' --exclude='a*' --include='b2' --exclude='b?' s3://mybucket.s3.us-east-2.amazonaws.com/ /mnt/jfs/

	Details: https://juicefs.com/docs/community/administration/sync
	Supported storage systems: https://juicefs.com/docs/community/how_to_setup_object_storage#supported-object-storage`
	app.Version = versioninfo.VERSION
	app.Copyright = "Apache License 2.0"
	app.Action = doSync
	app.Flags = addCategories("GENERAL", []cli.Flag{
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
	})
	app.Flags = append(app.Flags, addCategories("SELECTION", []cli.Flag{
		&cli.StringFlag{
			Name:    "start",
			Aliases: []string{"s"},
			Usage:   "the first `KEY` to sync",
		},
		&cli.StringFlag{
			Name:    "end",
			Aliases: []string{"e"},
			Usage:   "the last `KEY` to sync",
		},
		&cli.StringSliceFlag{
			Name:  "exclude",
			Usage: "exclude Key matching PATTERN",
		},
		&cli.StringSliceFlag{
			Name:  "include",
			Usage: "don't exclude Key matching PATTERN, need to be used with \"--exclude\" option",
		},
		&cli.Int64Flag{
			Name:  "limit",
			Usage: "limit the number of objects that will be processed (-1 is unlimited, 0 is to process nothing)",
			Value: -1,
		},
		&cli.BoolFlag{
			Name:    "update",
			Aliases: []string{"u"},
			Usage:   "skip files if the destination is newer",
		},
		&cli.BoolFlag{
			Name:    "force-update",
			Aliases: []string{"f"},
			Usage:   "always update existing files",
		},
		&cli.BoolFlag{
			Name:    "existing",
			Aliases: []string{"ignore-non-existing"},
			Usage:   "skip creating new files on destination",
		},
		&cli.BoolFlag{
			Name:  "ignore-existing",
			Usage: "skip updating files that already exist on destination",
		},
	})...)
	app.Flags = append(app.Flags, addCategories("ACTION", []cli.Flag{
		&cli.BoolFlag{
			Name:  "dirs",
			Usage: "sync directories or holders",
		},
		&cli.BoolFlag{
			Name:  "perms",
			Usage: "preserve permissions",
		},
		&cli.BoolFlag{
			Name:    "links",
			Aliases: []string{"l"},
			Usage:   "copy symlinks as symlinks",
		},
		&cli.BoolFlag{
			Name:    "delete-src",
			Aliases: []string{"deleteSrc"},
			Usage:   "delete objects from source those already exist in destination",
		},
		&cli.BoolFlag{
			Name:    "delete-dst",
			Aliases: []string{"deleteDst"},
			Usage:   "delete extraneous objects from destination",
		},
		&cli.BoolFlag{
			Name:  "check-all",
			Usage: "verify integrity of all files in source and destination",
		},
		&cli.BoolFlag{
			Name:  "check-new",
			Usage: "verify integrity of newly copied files",
		},
		&cli.BoolFlag{
			Name:  "dry",
			Usage: "don't copy file",
		},
	})...)
	app.Flags = append(app.Flags, addCategories("STORAGE", []cli.Flag{
		&cli.IntFlag{
			Name:    "threads",
			Aliases: []string{"p"},
			Value:   10,
			Usage:   "number of concurrent threads",
		},
		&cli.IntFlag{
			Name:  "list-threads",
			Value: 1,
			Usage: "number of threads to list objects",
		},
		&cli.IntFlag{
			Name:  "list-depth",
			Value: 1,
			Usage: "list the top N level of directories in parallel",
		},
		&cli.BoolFlag{
			Name:  "no-https",
			Usage: "donot use HTTPS",
		},
		&cli.StringFlag{
			Name:  "storage-class",
			Usage: "the storage class for destination",
		},
		&cli.IntFlag{
			Name:  "bwlimit",
			Usage: "limit bandwidth in Mbps (0 means unlimited)",
		},
	})...)
	app.Flags = append(app.Flags, addCategories("CLUSTER", []cli.Flag{
		&cli.StringFlag{
			Name:   "manager",
			Usage:  "the manager address used only by the worker node",
			Hidden: true,
		},
		&cli.StringSliceFlag{
			Name:  "worker",
			Usage: "hosts (separated by comma) to launch worker",
		},
		&cli.StringFlag{
			Name:  "manager-addr",
			Usage: "the IP address to communicate with workers",
		},
	})...)

	err := app.Run(reorderOptions(app, os.Args))
	if err != nil {
		logger.Fatalf("Error running juicesync: %s", err)
	}
}
