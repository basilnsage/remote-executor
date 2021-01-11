package utils

import (
	"bufio"
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

// SSH utilities

// NewSSHConfig: take in some common arguments and return an already-populated ssh.ClientConfig
func NewSSHConfig(checkHostKey bool, knownHostsFile, privateKeyFile, remoteUser string) (ssh.ClientConfig, error) {
	var conf ssh.ClientConfig
	var callback ssh.HostKeyCallback

	if checkHostKey {
		if cb, err := knownhosts.New(knownHostsFile); err != nil {
			return conf, fmt.Errorf("knowhosts.New: %v", err)
		} else {
			callback = cb
		}
	} else {
		callback = ssh.InsecureIgnoreHostKey()
	}

	pkey, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return conf, fmt.Errorf("ioutil.ReadFile: %v", err)
	}
	signer, err := ssh.ParsePrivateKey(pkey)
	if err != nil {
		return conf, fmt.Errorf("ssh.ParsePrivateKey: %v", err)
	}

	return ssh.ClientConfig{
		User:            remoteUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: callback,
	}, nil
}

// hosts parsing utilities

// ParseHostsList: uses the provided regex and formatter to return a list of hosts
// regex interprets the first grouping as the host string to format + return
func ParseHostsList(path string, re *regexp.Regexp, formatter func(string) string) ([]string, error) {
	var hosts []string

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open host list file: %v", err)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		matches := re.FindSubmatch(scanner.Bytes())
		if matches != nil {
			host := formatter(string(matches[1]))
			hosts = append(hosts, host)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %v", err)
	}
	return hosts, nil
}

// Append22: return the host string with `:22` appended if not already present
func Append22(host string) string {
	parts := strings.Split(host, ":")
	res := host
	if len(parts) == 1 && parts[0] != "" {
		res = fmt.Sprintf("%s:%d", host, 22)
	} else if len(parts) > 1 {
		last := parts[len(parts)-1]
		switch {
		case last == "":
			res = fmt.Sprintf("%s%d", host, 22)
		case last != "22":
			res = fmt.Sprintf("%s:%d", host, 22)
		default:
		}
	}
	return res
}

// logging utilities

type SyncLogger struct {
	Logger *log.Logger
	mu     sync.Mutex
}

func (l *SyncLogger) Info(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Logger.Printf("INFO: %s", msg)
}

func (l *SyncLogger) Error(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Logger.Printf("ERROR: %s", msg)
}

func (l *SyncLogger) Fatal(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Logger.Fatalf("FATAL: %s", msg)
}
