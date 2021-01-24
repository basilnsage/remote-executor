package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"

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

type failedHosts struct {
	failed []string
	mu     sync.Mutex
}

func newFailedHosts() *failedHosts {
	var hosts []string
	return &failedHosts{failed: hosts}
}

func (fh *failedHosts) append(host string) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.failed = append(fh.failed, host)
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
	pool := api.CreatePool(numWorkers, remoteCommand, sshConf)

	// schedule workers
	pool.ScheduleWorkers()

	fh := newFailedHosts()

	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(h string) {
			ctx := context.Background()
			res, err := pool.RunJob(ctx, h)
			if err != nil {
				syncLogger.Error(fmt.Sprintf("error running command against host: %s, error: %v", h, err))
				fh.append(h)
			} else if res.Err != nil {
				syncLogger.Error(fmt.Sprintf("%s\n%s\n%s", res.Host, res.Err.Error(), string(res.Output)))
				fh.append(h)
			} else {
				syncLogger.Info(string(res.Output))
			}
			wg.Done()
		}(host)
	}
	wg.Wait()

	if summarize && len(fh.failed) > 0 {
		logMsg := fmt.Sprintf("failed hosts:\n%s", strings.Join(fh.failed, "\n"))
		syncLogger.Info(logMsg)
	}
}
