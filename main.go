package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/basilnsage/remote-executor/api"
	"github.com/basilnsage/remote-executor/utils"
)

var (
	numWorkers     int
	checkHostKey   bool
	regexExpr      string
	remoteUser     string
	privateKeyPath string
	knownHostsPath string
	summarize      bool
)

func init() {
	homeDir, _ := os.LookupEnv("HOME")
	userName, _ := os.LookupEnv("USER")

	flag.IntVar(&numWorkers, "concurrency", 100, "size of worker pool")
	flag.BoolVar(&checkHostKey, "check-hostkey", false, "check remote host key")
	flag.StringVar(
		&regexExpr,
		"parser",
		`^([^\s]*)\b`,
		"regex used to parse host list",
	)
	flag.StringVar(&remoteUser, "user", userName, "remote user")
	flag.StringVar(
		&privateKeyPath,
		"private-key",
		fmt.Sprintf("%s/.ssh/id_rsa", homeDir),
		"ssh private key to use",
	)
	flag.StringVar(
		&knownHostsPath,
		"known-hosts",
		fmt.Sprintf("%s/.ssh/known_hosts", homeDir),
		"path to known hosts file",
	)
	flag.BoolVar(&summarize, "summarize", false, "report a list of failed hosts")
}

func main() {
	syncLogger := utils.SyncLogger{
		Logger: log.New(os.Stdout, "remote-executor: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
	}
	syncLogger.Info("starting new remote executor run")

	// parse flags and check positional arguments
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		syncLogger.Fatal(fmt.Sprintf("need 2 positional arguments, found: %d", len(args)))
	}
	hostList := args[0]
	remoteCommand := args[1]

	// create ssh client config

	sshConf, err := utils.NewSSHConfig(checkHostKey, knownHostsPath, privateKeyPath, remoteUser)
	if err != nil {
		syncLogger.Fatal(fmt.Sprintf("unable to parse flags: %v", err))
	}

	// compile re
	re, err := regexp.Compile(regexExpr)
	if err != nil {
		syncLogger.Fatal(fmt.Sprintf("unable to compile regex: %v", err))
	}

	// parse the host list
	hosts, err := utils.ParseHostsList(hostList, re, utils.Append22)
	if err != nil {
		syncLogger.Fatal(fmt.Sprintf("unable to parse host list: %v", err))
	}

	// create worker pool
	pool := api.CreatePool(numWorkers, len(hosts), remoteCommand, sshConf)

	// schedule workers
	pool.ScheduleWorkers()

	// schedule jobs; i.e. run remoteCommand against hosts
	go pool.ScheduleJobs(hosts)

	// collect and log results
	var failed []string
	results := make(chan api.Result, len(hosts))
	go func() {
		pool.StreamResults(results)
		close(results)
	}()
	for res := range results {
		if res.Err != nil {
			syncLogger.Error(fmt.Sprintf("%s\n%s\n%s", res.Host, res.Err.Error(), string(res.Output)))
			if summarize {
				failed = append(failed, res.Host)
			}
		} else {
			syncLogger.Info(string(res.Output))
		}
	}
	if summarize {
		logMsg := fmt.Sprintf("failed hosts:\n%s", strings.Join(failed, "\n"))
		syncLogger.Info(logMsg)
	}
}
