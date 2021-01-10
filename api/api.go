package api

import (
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
)

type WorkerPool struct {
	nWorkers    int
	jobs        chan string
	results     chan Result
	done        chan bool
	cmd         string
	sshConfig   ssh.ClientConfig
	wg          sync.WaitGroup
	isReturning sync.Mutex
}

func CreatePool(size int, cmd string, config ssh.ClientConfig) *WorkerPool {
	return &WorkerPool{
		nWorkers:  size,
		jobs:      make(chan string, 2*size),
		results:   make(chan Result, 2*size),
		done:      make(chan bool),
		cmd:       cmd,
		sshConfig: config,
	}
}

func (wp *WorkerPool) ScheduleWorkers() {
	for i := 0; i < wp.nWorkers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
	wp.wg.Wait()
	wp.done <- true
}

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

func (wp *WorkerPool) ScheduleJobs(hosts []string) {
	for _, host := range hosts {
		wp.jobs <- host
	}
	close(wp.jobs)
}

func (wp *WorkerPool) WaitAndReturnResults() []Result {
	var results []Result
	wp.isReturning.Lock()
	defer wp.isReturning.Unlock()

	for {
		select {
		case res := <-wp.results:
			results = append(results, res)
		case <-wp.done:
			return results
		}
	}
}

func (wp *WorkerPool) StreamResults(receiver chan<- Result) {
	wp.isReturning.Lock()
	defer wp.isReturning.Unlock()

	for {
		select {
		case res := <-wp.results:
			receiver <- res
		case <-wp.done:
			close(receiver)
			return
		}
	}
}

type Result struct {
	Host   string
	Output []byte
	Err    error
}