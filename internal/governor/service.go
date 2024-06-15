package governor

import (
	"context"
	fmtlib "fmt"
	"log"
	"time"
)

// Service interface are the methods a service must implement.
// The service should also embed `ServiceCtx` in the service struct.
type Service interface {

	// Run is the main service goroutine.
	//
	// The Governor calls this on a new goroutine when the service
	// is started. When Run() returns, the service will stop
	// and will be restarted if configured to do so.
	// The Governor wraps Run() calls with recover() and will catch
	// any panic() and log the error, then stop/restart the service.
	//
	// Run() should use the ServiceCtx.Ctx with any blocking
	// calls that can take a Context, to support cancellation.
	//
	// Network connections do not use a Context for reads and writes
	// (but they do for Dial calls) so the service should implement
	// the Stoppable interface to close any open net.Conn connections.
	//
	// Use ServiceCtx.Sleep() instead of time.Sleep() to support cancellation.
	Run()
}

type Stoppable interface {

	// Stop is called to stop the service (e.g. during shutdown)
	//
	// This is called when the Governor is trying to stop the
	// service, after the service Context is cancelled.
	//
	// NOTE: this will be called from a goroutine other than
	//       the service goroutine (may need a sync.Mutex)
	//
	// NOTE: be aware that this can be called while your service
	//       is starting up (i.e. around the time Run() is called!)
	//
	// Stop can be used to e.g. close network sockets that the
	// service might be blocking on (in Go, blocking network
	// calls ignore Context cancellation, so this is necessary
	// to interrupt those calls.)
	Stop()
}

// ServiceCtx is embedded in all service implementation-structs
// to provide a Context and common helper functions.
type ServiceCtx struct {
	ServiceName string
	Context     context.Context
}

// private interface to allow governor to init the ServiceCtx
type serviceContextInit interface {
	initServiceContextFromGovernor(ctx context.Context, name string)
}

func (s *ServiceCtx) initServiceContextFromGovernor(ctx context.Context, name string) {
	s.Context = ctx
	s.ServiceName = name
}

// Stopping returns true if the servce has been asked to stop:
// caller should exit the goroutine.
func (s *ServiceCtx) Stopping() bool {
	return s.Context.Err() != nil // Canceled or DeadlineExceeded
}

// Sleep returns true if it was interrupted by Governor asking
// the service to stop: caller should exit the service goroutine.
func (s *ServiceCtx) Sleep(duration time.Duration) bool {
	select {
	case <-s.Context.Done(): // Canceled or DeadlineExceeded
		return true
	case <-time.After(duration):
		return false
	}
}

// Log a message tagged with the service name.
func (s *ServiceCtx) Log(fmt string, args ...interface{}) {
	fmt = fmtlib.Sprintf("[%s] %s\n", s.ServiceName, fmt)
	log.Printf(fmt, args...)
}
