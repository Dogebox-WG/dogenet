package governor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Governor oversees all services and manages shutdown.
type Governor interface {

	// CatchSignals configures Governor to catch SIGINT (Ctrl+C)
	// and other interrupt signals (SIGTERM, SIGHUP, SIGQUIT)
	// and call Shutdown() when a signal is received.
	CatchSignals() Governor

	// Restart configures a default restart policy for all services,
	// which can be overridden on a per-service basis.
	// The restart delay can be zero.
	// Services never restart unless restart policy is set.
	Restart(after time.Duration) Governor

	// Add a service to Governor.
	// Returns a ServiceConfig interface to allow configuring and
	// starting the service.
	Add(name string, svc Service) ServiceConfig

	// Start all added services.
	// Does not affect services that have already been started.
	Start()

	// Shutdown stops all services.
	// This call blocks until all services have stopped.
	Shutdown()

	// WaitForShutdown waits for all services to be stopped.
	// Caveat: will return immediately if no services have been started.
	// This can be useful at the end of main()
	WaitForShutdown()
}

// ServiceConfig allows you to configure a service when adding
// it to the Governor.
type ServiceConfig interface {

	// Restart the service if it stops.
	// The restart delay can be zero.
	// This overrides the Governor restart policy.
	Restart(after time.Duration) ServiceConfig

	// Prevent service restart, to override Governor policy.
	// Services never restart unless a restart policy is set.
	NoRestart() ServiceConfig

	// Start the service immediately.
	// No more configuration is possible after calling Start()
	Start()
}

// StopService is a sentinel value that can be used stop a service
// from deep within a call stack by calling panic(StopService{}).
// Services are not stopped until they return from their Run() function;
// this provides an escape hatch for large, complex services.
type StopService struct{}

type governor struct {
	wg       sync.WaitGroup     // used to wait for all services to call Stopped()
	ctx      context.Context    // root context for new services
	cancel   context.CancelFunc // used to cancel all service contexts
	mutex    sync.Mutex         // protects the following members:
	services []*service
	delay    time.Duration
	stopping bool
	restart  bool
}

// New creates a new Governor to manage services.
func New() Governor {
	ctx, cancel := context.WithCancel(context.Background())
	g := &governor{
		ctx: ctx, cancel: cancel,
	}
	return g
}

func (g *governor) CatchSignals() Governor {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		<-signals
		log.Println("")
		log.Println("Shutdown requested via signal")
		g.Shutdown()
	}()
	return g
}

func (g *governor) Restart(after time.Duration) Governor {
	g.delay = after
	g.restart = true
	return g
}

func (g *governor) Add(name string, svc Service) ServiceConfig {
	// protect against a race with Shutdown
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.stopping {
		return &service{}
	}
	sctx := &service{g: g, svc: svc, delay: g.delay, restart: g.restart, name: name}
	g.services = append(g.services, sctx)
	return sctx
}

func (g *governor) Start() {
	// protect against a race with Shutdown
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if g.stopping {
		return
	}
	for _, svc := range g.services {
		svc.Start()
	}
}

func (g *governor) Shutdown() {
	// protect against a race with CatchSignals() goroutine
	g.mutex.Lock()
	if g.stopping {
		g.mutex.Unlock()
		return
	}
	g.stopping = true
	g.mutex.Unlock()
	// no need to hold the mutex below (Add calls are blocked)
	log.Println("Beginning shutdown…")
	g.cancel() // cancel all child contexts
	for _, svc := range g.services {
		// if the service has a Stop() function, call it
		if stopper, ok := svc.svc.(Stoppable); ok {
			stopper.Stop()
		}
	}
	g.wg.Wait()
	log.Println("Shutdown complete.")
}

func (g *governor) WaitForShutdown() {
	g.wg.Wait()
}

func (g *governor) is_stopping() bool {
	// protect against a race with Shutdown
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.stopping
}

// Service Context provides ServiceAPI for one service.
type service struct {
	g       *governor
	ctx     context.Context // one per service (so we can stop/restart individual services)
	cancel  context.CancelFunc
	svc     Service
	name    string
	delay   time.Duration
	restart bool
}

func (c *service) Name(name string) ServiceConfig {
	c.name = name
	return c
}

func (c *service) Restart(after time.Duration) ServiceConfig {
	c.delay = after
	c.restart = true
	return c
}

func (c *service) NoRestart() ServiceConfig {
	c.restart = false
	return c
}

func (c *service) Start() {
	if c.ctx != nil {
		return // already running
	}
	g := c.g
	g.wg.Add(1)
	c.ctx, c.cancel = context.WithCancel(g.ctx)
	go func() {
		for {
			c.service_run()
			if !g.is_stopping() && c.restart {
				log.Printf("[%s] service stopped, restarting in %v\n", c.name, c.delay)
				c.sleep(c.delay)
			} else {
				log.Printf("[%s] service stopped.\n", c.name)
				g.wg.Done()
				return
			}
		}
	}()
}

func (c *service) service_run() {
	defer func() {
		if err := recover(); err != nil {
			// don't log if the panic value is StopImmediate{}
			if _, ok := err.(StopService); ok {
				log.Printf("[%s] raised stop-immediate.\n", c.name)
			} else {
				log.Printf("[%s] crashed! error: %v\n", c.name, err)
			}
		}
	}()
	log.Printf("[%s] service starting.\n", c.name)
	if init, ok := c.svc.(serviceContextInit); ok {
		init.initServiceContextFromGovernor(c.ctx, c.name)
	} else {
		panic(fmt.Sprintf("[%s] bug: service does not embed governor.ServiceCtx", c.name))
	}
	c.svc.Run()
}

func (c *service) sleep(duration time.Duration) (cancelled bool) {
	select {
	case <-c.ctx.Done(): // receive context cancel
		return true
	case <-time.After(duration):
		return false
	}
}
