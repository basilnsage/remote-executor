package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const usage = "./remote-executor [--concurrency=100] [--check-hostkey=true] [--parser='foobar'] path_to_host_list cmd_to_run"

type config struct {
	numWorkers      int
	hostkeyCallback ssh.HostKeyCallback
	hostlist        string
	sshPort string
	hostlistRegex   *regexp.Regexp
	remoteCmd       string
	remoteUser      string
	privateKeyFile  string
}

var (
	numWorkers   int
	checkHostKey bool
	regexExpr    string
	remoteUser   string
	pkeyPath     string
	summarize bool
	port string
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
	flag.BoolVar(&summarize, "summarize", false, "report failed hosts at the end of the run")
	flag.StringVar(&port, "remote-port", "22", "remote server's SSH port")
}

// parseFlags: parse CLI flags to tune runetime behavior
func parseFlags(args []string) (*config, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("need 2 positional arguments, usage: %v", usage)
	}

	var callback ssh.HostKeyCallback
	if checkHostKey {
		homeDir, _ := os.LookupEnv("HOME")
		if hostkeyCallback, err := knownhosts.New(fmt.Sprintf("%s/.ssh/known_hosts", homeDir)); err != nil {
			return nil, fmt.Errorf("knowhosts.New: %v", err)
		} else {
			callback = hostkeyCallback
		}
	} else {
		callback = ssh.InsecureIgnoreHostKey()
	}
	re, err := regexp.Compile(regexExpr)
	if err != nil {
		return nil, fmt.Errorf("regexp.Compile: %v", err)
	}

	return &config{
		numWorkers:      numWorkers,
		hostkeyCallback: callback,
		hostlist:        args[0],
		sshPort: port,
		hostlistRegex:   re,
		remoteCmd:       args[1],
		remoteUser:      remoteUser,
		privateKeyFile:  pkeyPath,
	}, nil
}

type result struct {
	host string
	output []byte
	err error
}

type remoteCommand struct {
	cmd       string
	sshConfig ssh.ClientConfig
	jobs      chan string
	results   chan result
	errors    chan error
}

func newRemoteCommand(c *config) (*remoteCommand, error) {
	// parse private key to create a public key auth mechanism
	pkey, err := ioutil.ReadFile(c.privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key file: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(pkey)
	if err != nil {
		return nil, fmt.Errorf("could not parse private key: %v", err)
	}

	return &remoteCommand{
		cmd: c.remoteCmd,
		sshConfig: ssh.ClientConfig{
			User: c.remoteUser,
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(signer),
			},
			HostKeyCallback: c.hostkeyCallback,
		},
		jobs:    make(chan string, 2*c.numWorkers),
		results: make(chan result, 2*c.numWorkers),
		errors:  make(chan error, 2*c.numWorkers),
	}, nil
}

// acceptHosts: listen for hosts to run the remote command against
// report errors, successes back into the remoteCommand's channels
func (r *remoteCommand) worker(wg *sync.WaitGroup) {
	executor := func(host string) ([]byte, error) {
		client, err := ssh.Dial("tcp", host, &r.sshConfig)
		if err != nil {
			return nil, fmt.Errorf("could not dial: %v", err)
		}

		sess, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("unable to create session: %v", err)
		}
		defer func() { _ = sess.Close() }()

		return sess.CombinedOutput(r.cmd)
	}
	for host := range r.jobs {
		output, err := executor(host)
		r.results <- result{
			host,
			output,
			err,
		}
	}
	wg.Done()
}

// queueWorkers: start worker goroutines that will execute the remote command
func (r *remoteCommand) queueWorkers(n int, done chan<- bool) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go r.worker(&wg)
	}
	wg.Wait()
	done <- true
}

// schedule: take in a list of hosts and schedule running the remote command
// against each host
func (r *remoteCommand) schedule(hosts []string, remotePort string) {
	for _, host := range hosts {
		r.jobs <- fmt.Sprintf("%s:%s", host, remotePort)
	}
	close(r.jobs)
}

// parseHosts: parse the provided host list to identify hosts to run the remote command against
func parseHosts(hostFile string, re *regexp.Regexp) ([]string, error) {
	hosts := make([]string, 0, 0)

	file, err := os.Open(hostFile)
	if err != nil {
		return nil, fmt.Errorf("unable to open host list file: %v", err)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		matches := re.FindSubmatch(scanner.Bytes())
		if matches != nil {
			hosts = append(hosts, string(matches[1]))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %v", err)
	}
	return hosts, nil
}

type logger struct {
	l  *log.Logger
	mu sync.Mutex
}

func (l *logger) info(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Printf("INFO: %s", msg)
}

func (l *logger) error(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.l.Printf("ERROR: %s", msg)
}

func main() {
	l := &logger{
		l: log.New(os.Stdout, "remote-executor: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
	}

	// parse flags and create configs
	flag.Parse()
	conf, err := parseFlags(flag.Args())
	if err != nil {
		log.Fatalf("unable to parse flags: %v", err)
	}

	// create a new remoteCommand object to manage execution
	rc, err := newRemoteCommand(conf)
	if err != nil {
		log.Fatalf("unable to initialize the remote command: %v", err)
	}

	// parse host list
	hosts, err := parseHosts(conf.hostlist, conf.hostlistRegex)
	if err != nil {
		log.Fatalf("unable to parse host list: %v", err)
	}

	// schedule workers
	done := make(chan bool)
	l.info("queueing workers")
	go rc.queueWorkers(conf.numWorkers, done)

	// create jobs
	l.info("scheduling jobs")
	go rc.schedule(hosts, conf.sshPort)

	// parse results -- block on this
	failed := make([]string, 0)
	l.info("waiting for results")
	for {
		select {
		case res := <-rc.results:
			if res.err != nil {
				l.error(fmt.Sprintf("%s\n%s", res.err.Error(), string(res.output)))
				failed = append(failed, res.host)
			} else {
				l.info(string(res.output))
			}
		case <-done:
			if summarize && len(failed) > 0 {
				l.info(fmt.Sprintf("failed hosts:\n%s", strings.Join(failed, "\n")))
			}
			l.info("exiting...")
			return
		}
	}
}
