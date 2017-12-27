package main

import (
	"sync"

	"github.com/miekg/dns"
)

var (
	MaxWorker = 1000
	MaxQueue  = 1000
)

// Job represents the job to be run
type Job struct {
	Msg        *dns.Msg
	ResultChan chan<- Result
}

// A buffered channel that we can send work requests on.
var JobQueue chan Job = make(chan Job)

// Worker represents the worker that executes the job
type Worker struct {
	WorkerPool   chan chan Job
	JobChannel   chan Job
	oneJobDoneWg *sync.WaitGroup
	//quit       chan bool
}

func NewWorker(workerPool chan chan Job, wg *sync.WaitGroup) Worker {
	return Worker{
		WorkerPool:   workerPool,
		JobChannel:   make(chan Job),
		oneJobDoneWg: wg}
}

// Start method starts the run loop for the worker, listening for a quit channel in
// case we need to stop it
func (w Worker) Start(wg *sync.WaitGroup) {
	go func() {
		defer wg.Done()
		for {
			// register the current worker into the worker queue.
			w.WorkerPool <- w.JobChannel

			select {
			case job, ok := <-w.JobChannel:
				if ok {
					// work on job
					job.queryAndMesaure()
					w.oneJobDoneWg.Done()
				} else {
					return
				}
			}
		}
	}()
}

// Stop signals the worker to stop listening for work requests.
func (w *Worker) Stop() {
	close(w.JobChannel)
}

type Dispatcher struct {
	// A pool of workers channels that are registered with the dispatcher
	WorkerPool      chan chan Job
	WorkerList      []*Worker
	workerAllDoneWg *sync.WaitGroup
}

// NewDispatcher creates new Dispatcher
func NewDispatcher(maxWorkers int) *Dispatcher {
	pool := make(chan chan Job, maxWorkers)
	return &Dispatcher{WorkerPool: pool, WorkerList: make([]*Worker, 0, maxWorkers), workerAllDoneWg: new(sync.WaitGroup)}
}

// Run creates workers and starts dispatching job
func (d *Dispatcher) Run(wg *sync.WaitGroup) {
	// starting n number of workers
	for i := 0; i < MaxWorker; i++ {
		worker := NewWorker(d.WorkerPool, d.workerAllDoneWg)
		d.WorkerList = append(d.WorkerList, &worker)
		wg.Add(1)
		worker.Start(wg)
	}

	go d.dispatch()
}

// Stop stops every working that are working, call this method after stop receiving requests
func (d *Dispatcher) Stop() {
	for _, worker := range d.WorkerList {
		worker.Stop()
	}
}

func (d *Dispatcher) dispatch() {
	exit := false
	for {
		select {
		case job, ok := <-JobQueue:
			if ok {
				// a job request has been received
				go func(job Job) {
					// try to obtain a worker job channel that is available.
					// this will block until a worker is idle
					jobChannel := <-d.WorkerPool

					// dispatch the job to the worker job channel
					d.workerAllDoneWg.Add(1)
					jobChannel <- job
				}(job)
			} else {
				exit = true
			}
		}
		if exit {
			break
		}
	}
	d.workerAllDoneWg.Wait()
	d.Stop()
	return
}
