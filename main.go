package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/basilnsage/remote-executor/api"
	"github.com/basilnsage/remote-executor/utils"
)

var (
	numWorkers     int
	checkHostKey   bool
	regexExpr      string
	remoteUser     string
	pkeyPath       string
	knownHostsPath string
)

func init() {
	homeDir, _ := os.LookupEnv("HOME")
	userName, _ := os.LookupEnv("USER")

	flag.IntVar(&numWorkers, "concurrency", 100, "size of worker pool")
	flag.BoolVar(&checkHostKey, "check-hostkey", true, "check remote host key")
	flag.StringVar(
		&regexExpr,
		"parser",
		`^(.*)\b`,
		"regex used to parse host list",
	)
	flag.StringVar(&remoteUser, "user", userName, "remote user")
	flag.StringVar(
		&pkeyPath,
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
		syncLogger.Fatal(fmt.Sprintf("need 2 positional arugments, found: %d", len(args)))
	}
	hostList := args[0]
	remoteCommand := args[1]

	// create ssh client config
	sshConf, err := utils.NewSSHConfig(checkHostKey, knownHostsPath, pkeyPath, remoteUser)
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
	pool := api.CreatePool(numWorkers, remoteCommand, sshConf)

	// schedule workers
	go pool.ScheduleWorkers()

	// schedule jobs; i.e. run remoteCommand against hosts
	go pool.ScheduleJobs(hosts)

	// collect and log results
	results := make(chan api.Result, len(hosts))
	go pool.StreamResults(results)
	for res := range results {
		if res.Err != nil {
			syncLogger.Error(fmt.Sprintf("%s\n%s\n%s", res.Host, res.Err.Error(), string(res.Output)))
		} else {
			syncLogger.Info(string(res.Output))
		}
	}
}
