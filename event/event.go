package event

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrorHandler is the error handler.
	// If you want to use your own error handler, please set to ErrorHandler.
	ErrorHandler = func(err interface{}) {}

	// ErrDone represents that a queue is finished. This is used for internal control.
	ErrDone = errors.New("queue is done")

	// ErrNotExist is passed to ErrorHandler if handler not exists.
	ErrNotExist = errors.New("handler not exist")

	workersPerQueue   = 1
	queues            = make(map[string]Queue)
	handlerQueueNames = make(map[string]handlerQueueName)
	workers           []*worker
	wg                = struct{ enqueue, dequeue sync.WaitGroup }{}
)

// AddHandler adds handler that related to name and queue.
// The name is an event name such as "log.error" that will be used for Trigger.
// The queueName is a name of queue registered by RegisterQueue in advance.
// If queue of queueName still hasn't been registered, it returns error.
// Also it returns error if you try to add same handler name twice.
func AddHandler(name string, queueName string, handler func(args ...interface{}) error) error {
	queue := queues[queueName]
	if queue == nil {
		return fmt.Errorf("kocha: event: queue `%s' isn't registered", queueName)
	}
	if _, exist := handlerQueueNames[name]; exist {
		return fmt.Errorf("kocha: event: AddHandler called twice for handler %s", name)
	}
	handlerQueueNames[name] = handlerQueueName{handler, queueName}
	return nil
}

// Trigger emits the event.
// The name is an event name. It must be added in advance using AddHandler.
// If Trigger called by not added name, it returns error.
// If args are given, they will be passed to handler added by AddHandler.
func Trigger(name string, args ...interface{}) error {
	hq, exist := handlerQueueNames[name]
	if !exist {
		return fmt.Errorf("kocha: event: handler `%s' isn't added", name)
	}
	queue := queues[hq.queueName]
	wg.enqueue.Add(1)
	go func() {
		defer wg.enqueue.Done()
		defer func() {
			if err := recover(); err != nil {
				ErrorHandler(err)
			}
		}()
		if err := enqueue(queue, payload{name, args}); err != nil {
			panic(err)
		}
	}()
	return nil
}

type handlerQueueName struct {
	handler   handler
	queueName string
}

// alias.
type handler func(args ...interface{}) error

func enqueue(queue Queue, pld payload) error {
	var data string
	if err := pld.encode(&data); err != nil {
		return err
	}
	return queue.Enqueue(data)
}

// Start starts background event workers.
// By default, workers per queue is 1. To set the workers per queue, use
// SetWorkersPerQueue before Start calls.
func Start() {
	for name, queue := range queues {
		for i := 0; i < workersPerQueue; i++ {
			worker := newWorker(name, queue.New(workersPerQueue), handlerQueueNames, &wg.dequeue)
			workers = append(workers, worker)
			go worker.start()
		}
	}
}

// SetWorkersPerQueue sets the number of workers per queue.
// It must be called before Start calls.
func SetWorkersPerQueue(n int) {
	if n < 1 {
		n = 1
	}
	workersPerQueue = n
}

// Stop wait for all workers to complete.
func Stop() {
	wg.enqueue.Wait()
	defer func() {
		workers = nil
	}()
	defer wg.dequeue.Wait()
	for _, worker := range workers {
		worker.stop()
	}
}

// Queue is the interface that must be implemeted by background event queue.
type Queue interface {
	// New returns a new Queue to launch the workers.
	// You can use an argument n as a hint when you create a new queue.
	// n is the number of workers per queue.
	New(n int) Queue

	// Enqueue add data to the queue.
	Enqueue(data string) error

	// Dequeue returns the data that fetch from the queue.
	// It will return ErrDone as err when Stop is called.
	Dequeue() (data string, err error)

	// Stop wait for Enqueue and/or Dequeue to complete then will stop a queue.
	Stop()
}

// RegisterQueue makes a background queue available by the provided name.
// If queue is already registerd or if queue nil, it panics.
func RegisterQueue(name string, queue Queue) {
	if queue == nil {
		panic(fmt.Errorf("kocha: event: Register queue is nil"))
	}
	if _, exist := queues[name]; exist {
		panic(fmt.Errorf("kocha: event: Register queue `%s' is already registered", name))
	}
	queues[name] = queue
}
