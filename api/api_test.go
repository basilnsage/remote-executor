package api

import (
	"bytes"
	"context"
	cRand "crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/crypto/ssh"
)

var tests = map[string]struct {
	iterations int
	nWorkers   int
	hosts      []string
}{
	"small pool few jobs": {
		10,
		5,
		randHosts(5),
	},
	"med pool few jobs": {
		10,
		100,
		randHosts(5),
	},
	"small pool many jobs": {
		10,
		5,
		randHosts(500),
	},
	"med pool many jobs": {
		10,
		100,
		randHosts(500),
	},
}

func TestMainFlow(t *testing.T) {
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			var good, bad float64
			var toLog string
			for i := 0; i < test.iterations; i++ {
				wp := CreatePool(test.nWorkers, "noop", ssh.ClientConfig{})
				wp.do = wp.testWorker
				wp.ScheduleWorkers()
				var wg sync.WaitGroup
				for _, host := range test.hosts {
					wg.Add(1)
					go func(h string) {
						got, err := wp.RunJob(context.Background(), h)
						if err != nil {
							t.Errorf("RunJob: %v", err)
						}
						want := Result{
							h,
							[]byte("test"),
							nil,
						}
						if diff := cmp.Diff(got, want); diff != "" {
							mu.Lock()
							bad++
							toLog = diff
							mu.Unlock()
						}
						wg.Done()
					}(host)
				}
				wg.Wait()
			}
			if bad != 0 {
				percentPass := 100.0 * (good / (good + bad))
				t.Fatalf("%g percent of attempts correct, last diff: %s", percentPass, toLog)
			}
		})
	}
}

func TestExecutor(t *testing.T) {
	b := make([]byte, 32)
	_, err := cRand.Read(b)
	if err != nil {
		t.Fatalf("crypto/rand.Read: %v", err)
	}

	clientConf := ssh.ClientConfig{
		User:            "test",
		Auth:            []ssh.AuthMethod{ssh.Password(string(b))},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	done := make(chan struct{})
	ready := make(chan struct{})
	go func() {
		if err := newSSHServer(b, done, ready); err != nil {
			t.Fatalf("issue running SSH server: %v", err)
		}
	}()
	<-ready
	wp1 := CreatePool(10, "test", clientConf)
	output, err := wp1.executor("localhost:2022")
	if err != nil {
		t.Fatalf("executor failed: %v", err)
	}
	if got, want := string(output), "success!"; got != want {
		t.Fatalf("executor returned %v, want %v", got, want)
	}

	wp2 := CreatePool(10, "fail", clientConf)
	output, err = wp2.executor("localhost:2022")
	if err != nil && err.Error() != "Process exited with status 1" {
		t.Fatalf("executor failed: %v", err)
	}
	if got, want := string(output), "failed!"; got != want {
		t.Fatalf("executor returned %v, want %v", got, want)
	}
	close(done)
}

//func newSSHServer(serverPass []byte, done <- chan struct{}) (*ssh.ServerConn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
func newSSHServer(serverPass []byte, done <-chan struct{}, ready chan<- struct{}) error {
	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "test" && subtle.ConstantTimeCompare(serverPass, pass) == 1 {
				return nil, nil
			} else {
				return nil, errors.New("unauthorized")
			}
		},
	}

	privateKey, _ := rsa.GenerateKey(cRand.Reader, 2048)
	privateKeyPEM := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(privateKey),
	}
	private, err := ssh.ParsePrivateKey(pem.EncodeToMemory(&privateKeyPEM))
	if err != nil {
		return fmt.Errorf("ParsePrivateKey: %v", err)
	}
	serverConfig.AddHostKey(private)

	listener, err := net.Listen("tcp", "localhost:2022")
	if err != nil {
		return fmt.Errorf("net.Listen: %v", err)
	}
	close(ready)

	for {
		// blocks waiting for connection
		nConn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("listener.Accept: %v", err)
		}

		conn, chans, reqs, err := ssh.NewServerConn(nConn, serverConfig)
		if err != nil {
			return fmt.Errorf("NewServerConn: %v", err)
		}

		go ssh.DiscardRequests(reqs)

		select {
		case newChannel := <-chans:
			if newChannel.ChannelType() != "session" {
				newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
				continue
			}
			channel, requests, err := newChannel.Accept()
			if err != nil {
				return fmt.Errorf("could not accept channel: %v", err)
			}

			go func(in <-chan *ssh.Request) {
				defer channel.Close()
				for req := range in {
					switch req.Type {
					case "exec":
						cmd := req.Payload[4:]
						var output, exitStatus []byte
						if string(cmd) == "test" {
							output = []byte("success!")
							exitStatus = []byte{0, 0, 0, 0}
						} else {
							output = []byte("failed!")
							exitStatus = []byte{0, 0, 0, 1}
						}
						if err := req.Reply(true, nil); err != nil {
							log.Fatalf("could not reply to request: %v", err)
						}

						if _, err := io.Copy(channel, bytes.NewReader(output)); err != nil {
							log.Fatalf("io.Copy: %v", err)
						}

						if ok, err := channel.SendRequest("exit-status", false, exitStatus); err != nil {
							log.Fatalf("could not send request to channel: %v, ok: %v", err, ok)
						}
						return
					}
				}
			}(requests)
		case <-done:
			return conn.Close()
		}
	}
}

func randHosts(n int) []string {
	var hosts []string
	for i := 0; i < n; i++ {
		hosts = append(hosts, strconv.Itoa(rand.Int()))
	}
	return hosts
}

func (wp *WorkerPool) testWorker() {
	for job := range wp.jobs {
		job.result.Host = job.host
		job.result.Output = []byte("test")
		job.result.Err = nil
		job.done <- struct{}{}
	}
	wp.wg.Done()
}
