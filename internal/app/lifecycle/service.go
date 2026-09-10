package lifecycle

import (
	"sync"
)

type (
	// Starter is the interface wraps the Start method.
	Starter interface {
		Start()
	}

	// Stopper is the interface wraps the Stop method.
	Stopper interface {
		Stop()
	}

	// Service is the interface that groups Start and Stop methods.
	Service interface {
		Starter
		Stopper
	}

	// Group A ServiceGroup is a group of services.
	// Attention: the starting order of the added services is not guaranteed.
	Group struct {
		services []Service
		stopOnce sync.Once
	}
)

// NewServiceGroup returns a ServiceGroup.
func NewServiceGroup() *Group {
	return new(Group)
}

// Add adds service into sg.
func (sg *Group) Add(service Service) {
	// push front, stop with reverse order.
	sg.services = append([]Service{service}, sg.services...)
}

// Start starts the ServiceGroup.
// There should not be any logic code after calling this method, because this method is a blocking one.
// Also, quitting this method will close the logx output.
func (sg *Group) Start() {
	AddShutdownListener(func() {
		sg.Stop()
	})

	sg.doStart()
}

// Stop stops the ServiceGroup.
func (sg *Group) Stop() {
	sg.stopOnce.Do(sg.doStop)
}

func (sg *Group) doStart() {
	var group sync.WaitGroup
	for _, service := range sg.services {
		group.Go(service.Start)
	}
	group.Wait()
}

func (sg *Group) doStop() {
	for _, service := range sg.services {
		service.Stop()
	}
}

// WithStart wraps a start func as a Service.
func WithStart(start func()) Service {
	return startOnlyService{
		start: start,
	}
}

// WithStarter wraps a Starter as a Service.
func WithStarter(start Starter) Service {
	return starterOnlyService{
		Starter: start,
	}
}

type (
	stopper struct{}

	startOnlyService struct {
		start func()
		stopper
	}

	starterOnlyService struct {
		Starter
		stopper
	}
)

func (s stopper) Stop() {
}

func (s startOnlyService) Start() {
	s.start()
}
