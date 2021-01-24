package api

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
)

// WorkerPool: everything required to orchestrate running the command against remote hosts
type WorkerPool struct {
	numWorkers int
	jobs       chan JobResult
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

type JobResult struct {
	host   string
	result *Result
	done   chan struct{}
}

// CreatePool: create the worker pool
func CreatePool(poolSize int, cmd string, config ssh.ClientConfig) *WorkerPool {
	res := &WorkerPool{
		numWorkers: poolSize,
		jobs:       make(chan JobResult),
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

// Connect to the remote server, execute the command, and return the output.
func (wp *WorkerPool) executor(host string) ([]byte, error) {
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

// This is the actual worker that does the actual work. worker establishes an SSH session with the remote host and
// runs the command on the remote host. It then waits for the result, an error if one is present, and adds a new
// Result to the wp.results channel.
// results will block if the channel is not made large enough or if results are not drained in a timely manner.
func (wp *WorkerPool) worker() {
	for job := range wp.jobs {
		output, err := wp.executor(job.host)
		job.result.Host = job.host
		job.result.Output = output
		job.result.Err = err
		close(job.done)
	}

	wp.wg.Done()
}

// RunJob: run the remote command against the specified host and return the Result.
// Return an error if the context is cancelled before the job finishes.
func (wp *WorkerPool) RunJob(ctx context.Context, host string) (Result, error) {
	res := new(Result)
	done := make(chan struct{})

	select {
	case wp.jobs <- JobResult{host, res, done}:
	case <-ctx.Done():
		return Result{}, nil
	}

	select {
	case <-done:
		return *res, nil
	case <-ctx.Done():
		return Result{}, nil
	}
}
