package astibrain

import (
	"context"
	"sync"

	"github.com/asticode/go-astilog"
	"github.com/pkg/errors"
)

// Initializable represents an object that can be initialized.
type Initializable interface {
	Init() error
}

// Activable represents an object that can be activated.
type Activable interface {
	Activate(a bool)
}

// Runnable represents an object that can be run.
type Runnable interface {
	Run(ctx context.Context) error
}

// AbilityOptions represents ability options
type AbilityOptions struct {
	AutoStart bool
}

// ability represents an ability.
type ability struct {
	a        interface{}
	cancel   context.CancelFunc
	chanDone chan error
	ctx      context.Context
	m        sync.Mutex
	name     string
	o        AbilityOptions
	ws       *webSocket
}

// newAbility creates a new ability.
func newAbility(name string, a interface{}, ws *webSocket, o AbilityOptions) *ability {
	return &ability{
		a:        a,
		chanDone: make(chan error),
		name:     name,
		o:        o,
		ws:       ws,
	}
}

// isOnUnsafe returns whether the ability is on while making the assumption that the mutex is locked.
func (a *ability) isOnUnsafe() bool {
	return a.ctx != nil && a.ctx.Err() == nil
}

// isOn returns whether the ability is on.
func (a *ability) isOn() bool {
	a.m.Lock()
	defer a.m.Unlock()
	return a.isOnUnsafe()
}

// on switches the ability on.
func (a *ability) on() {
	// Ability is already on
	if a.isOn() {
		return
	}

	// Log
	astilog.Debugf("astibrain: switching %s on", a.name)

	// Reset the context
	a.ctx, a.cancel = context.WithCancel(context.Background())

	// Wait for the end of execution in a go routine
	go a.wait()

	// Switch on the activity
	if v, ok := a.a.(Activable); ok {
		a.onActivable(v)
	} else if v, ok := a.a.(Runnable); ok {
		a.onRunnable(v)
	}

	// Log
	astilog.Infof("astibrain: %s have been switched on", a.name)

	// Dispatch websocket event
	a.ws.send(WebsocketEventNameAbilityStarted, a.name)
}

// onActivable switches the activable ability on.
func (a *ability) onActivable(v Activable) {
	// Activate
	v.Activate(true)

	// Listen to context in a goroutine
	go func() {
		<-a.ctx.Done()
		v.Activate(false)
		a.chanDone <- nil
	}()
}

// onRunnable switches the runnable ability on.
func (a *ability) onRunnable(v Runnable) {
	// Run in a goroutine
	go func() {
		a.chanDone <- v.Run(a.ctx)
	}()
}

// wait waits for the ability to stop or for the context to be done
func (a *ability) wait() {
	// Ability is not on
	if !a.isOn() {
		return
	}

	// Make sure the context is cancelled
	defer a.cancel()

	// Listen to chanDone
	if err := <-a.chanDone; a.ctx.Err() == nil {
		// Log
		astilog.Error(errors.Wrapf(err, "astibrain: %s crashed", a.name))

		// Dispatch websocket event
		a.ws.send(WebsocketEventNameAbilityCrashed, a.name)
	} else {
		// Log
		astilog.Infof("astibrain: %s have been switched off", a.name)

		// Dispatch websocket event
		a.ws.send(WebsocketEventNameAbilityStopped, a.name)
	}
	return
}

// off switches the ability off.
func (a *ability) off() {
	// Ability is already off
	if !a.isOn() {
		return
	}

	// Log
	astilog.Debugf("astibrain: switching %s off", a.name)

	// Switch off
	a.cancel()

	// The rest is handled through the wait function
}
