package api

import (
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
)

// WorkerPool: everything required to orchestrate running the command against remote hosts
type WorkerPool struct {
	numWorkers int
	jobs       chan string
	results    chan Result
	cmd        string
	sshConfig  ssh.ClientConfig
	wg         sync.WaitGroup
	do         func()
}

// Result: the results of running a command against a specific host.
// The struct and its fields are exported to enable live-streaming results to the caller.
type Result struct {
	Host   string
	Output []byte
	Err    error
}

// CreatePool: create the worker pool
func CreatePool(poolSize, queueLength int, cmd string, config ssh.ClientConfig) *WorkerPool {
	res := &WorkerPool{
		numWorkers: poolSize,
		jobs:       make(chan string, queueLength),
		results:    make(chan Result, queueLength),
		cmd:        cmd,
		sshConfig:  config,
	}
	res.do = res.worker
	return res
}

// ScheduleWorkers: add workers to the worker pool
func (wp *WorkerPool) ScheduleWorkers() {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		go wp.do()
	}
}

// This is the actual worker that does the actual work. worker establishes an SSH session with the remote host and
// runs the command on the remote host. It then waits for the result, an error if one is present, and adds a new
// Result to the wp.results channel.
// results will block if the channel is not made large enough or if results are not drained in a timely manner.
func (wp *WorkerPool) worker() {
	executor := func(host string) ([]byte, error) {
		client, err := ssh.Dial("tcp", host, &wp.sshConfig)
		if err != nil {
			return nil, fmt.Errorf("could not dial: %v", err)
		}

		sess, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("unable to create session: %v", err)
		}
		defer func() { _ = sess.Close() }()

		return sess.CombinedOutput(wp.cmd)
	}

	for host := range wp.jobs {
		output, err := executor(host)
		wp.results <- Result{
			host,
			output,
			err,
		}
	}

	wp.wg.Done()
}

// ScheduleJobs: run the command against all hosts
func (wp *WorkerPool) ScheduleJobs(hosts []string) {
	for _, host := range hosts {
		wp.jobs <- host
	}
	close(wp.jobs)
}

func (wp *WorkerPool) wait() {
	go func() {
		wp.wg.Wait()
		close(wp.results)
	}()
}

// Return methods: these all drain the results and should only be called once!
// Only one return method may be called for each worker pool. Multiple calls will try to close the wp.results channel
// twice resulting in a panic.

// Wait: wait for all jobs to finish and throw away the results
func (wp *WorkerPool) Wait() {
	wp.wait()
	for range wp.results {
	}
}

// WaitAndReturnResults: wait for all jobs to finish and return the collected results
func (wp *WorkerPool) WaitAndReturnResults() []Result {
	wp.wait()
	var results []Result
	for res := range wp.results {
		results = append(results, res)
	}
	return results
}

// StreamResults: stream live results back to the caller as soon as they are ready
func (wp *WorkerPool) StreamResults(receiver chan<- Result) {
	wp.wait()
	for res := range wp.results {
		receiver <- res
	}
}
