package api

import (
	"github.com/google/go-cmp/cmp"
	"golang.org/x/crypto/ssh"
	"math/rand"
	"sort"
	"strconv"
	"testing"
)

var tests = map[string]struct{
	iterations int
	nWorkers int
	hosts []string
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
			var good, bad float64
			var toLog string
			for i := 0; i < test.iterations; i++ {
				wp := CreatePool(test.nWorkers, "noop", ssh.ClientConfig{})
				wp.do = wp.testWorker
				wp.ScheduleWorkers()
				go wp.ScheduleJobs(test.hosts)
				{
					var got results = wp.WaitAndReturnResults()
					want := resultsFromHosts(test.hosts)
					sort.Sort(got)
					sort.Sort(want)
					if diff := cmp.Diff(got, want); diff != "" {
						bad++
						toLog = diff
					} else {
						good++
					}
				}
			}
			if bad != 0 {
				percentPass := 100.0 * (good / (good + bad))
				t.Fatalf("%g percent of attempts correct, last diff: %s", percentPass, toLog)
			}
		})
	}
}

func resultsFromHosts(hosts []string) results {
	var res results
	for _, host := range hosts {
		res = append(res, Result{
			host,
			[]byte("test"),
			nil,
		})
	}
	return res
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
		//time.Sleep(10 * time.Millisecond)
		wp.results <- Result{
			job,
			[]byte("test"),
			nil,
		}
	}
	wp.wg.Done()
}

type results []Result

func (r results) Len() int {
	return len(r)
}

func (r results) Less(i, j int) bool {
	return r[i].Host < r[j].Host
}

func (r results) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
