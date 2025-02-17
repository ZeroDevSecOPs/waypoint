package runner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/waypoint-plugin-sdk/component"
	"github.com/hashicorp/waypoint-plugin-sdk/terminal"
	"github.com/hashicorp/waypoint/internal/factory"
	"github.com/hashicorp/waypoint/internal/plugin"
	"github.com/hashicorp/waypoint/internal/server"
	pb "github.com/hashicorp/waypoint/internal/server/gen"
)

var (
	ErrClosed  = errors.New("runner is closed")
	ErrTimeout = errors.New("runner timed out waiting for a job")
)

const (
	// envLogLevel is the env var to set with the log level. This
	// env var matches the Waypoint CLI on purpose. This can be set on
	// the runner process OR via app config (`waypoint config`).
	envLogLevel = "WAYPOINT_LOG_LEVEL"
)

// Runners in Waypoint execute operations. These can be local (the CLI)
// or they can be remote (triggered by some webhook). In either case, they
// share this same underlying implementation.
//
// To use a runner:
//
//   1. Initialize it with New. This will setup some initial state but
//      will not register with the server or run jobs.
//
//   2. Start the runner with "Start". This will register the runner and
//      kick off some management goroutines. This will not execute any jobs.
//
//   3. Run a single job with "Accept". This is named to be similar to a
//      network listener "accepting" a connection. This will request a single
//      job from the Waypoint server, block until one is available, and execute
//      it. Repeat this call for however many jobs you want to execute.
//
//   4. Clean up with "Close". This will gracefully exit the runner, waiting
//      for any running jobs to finish.
//
type Runner struct {
	id          string
	logger      hclog.Logger
	client      pb.WaypointClient
	cleanupFunc func()
	runner      *pb.Runner
	factories   map[component.Type]*factory.Factory
	ui          terminal.UI
	local       bool
	tempDir     string

	// protects whether or not the runner is active or not.
	runningCond *sync.Cond
	shutdown    bool
	runningJobs int

	runningCtx    context.Context
	runningCancel func()

	enableDynConfig bool

	// stateCond and its associated locker are used to protect all the
	// state-prefixed fields. These state fields can be watched using this
	// cond for state changes in the runner. Anyone waiting on stateCond should
	// also verify the context didn't cancel. The stateCond will be broadcasted
	// when the root context cancels.
	stateCond       *sync.Cond
	stateConfig     bool // config stream is connected
	stateConfigOnce bool // true once we process config once, success or error
	stateJobReady   bool // ready to start accepting jobs
	stateExit       bool // true when exiting

	// config is the current runner config.
	config      *pb.RunnerConfig
	originalEnv []*pb.ConfigVar

	acceptTimeout time.Duration

	// configPlugins is the mapping of config source type to launched plugin.
	// Note this is not currently configurable and we just statically set
	// this to `plugin.ConfigSourcers`. Everything is set up so that
	// in the future it can be configurable though.
	configPlugins map[string]*plugin.Instance

	// noopCh is used in tests only. This will cause any noop operations
	// to block until this channel is closed.
	noopCh <-chan struct{}
}

// New initializes a new runner.
//
// You must call Start to start the runner and register with the Waypoint
// server. See the Runner struct docs for more details.
func New(opts ...Option) (*Runner, error) {
	// Our default runner
	runner := &Runner{
		logger: hclog.L(),
		factories: map[component.Type]*factory.Factory{
			component.MapperType:         plugin.BaseFactories[component.MapperType],
			component.BuilderType:        plugin.BaseFactories[component.BuilderType],
			component.RegistryType:       plugin.BaseFactories[component.RegistryType],
			component.PlatformType:       plugin.BaseFactories[component.PlatformType],
			component.ReleaseManagerType: plugin.BaseFactories[component.ReleaseManagerType],
			component.TaskLauncherType:   plugin.BaseFactories[component.TaskLauncherType],
		},
		stateCond: sync.NewCond(&sync.Mutex{}),
	}

	runner.runningCond = sync.NewCond(new(sync.Mutex))
	runner.runningCtx, runner.runningCancel = context.WithCancel(context.Background())

	// Setup our default config sourcers.
	runner.configPlugins = plugin.ConfigSourcers

	// Build our config
	var cfg config
	for _, o := range opts {
		err := o(runner, &cfg)
		if err != nil {
			return nil, err
		}
	}

	// If the options didn't populate id, then we do so now.
	if runner.id == "" {
		// Create our ID
		id, err := server.Id()
		if err != nil {
			return nil, status.Errorf(codes.Internal,
				"failed to generate unique ID: %s", err)
		}

		runner.id = id
	}

	runner.runner = &pb.Runner{
		Id:       runner.id,
		ByIdOnly: cfg.byIdOnly,
		Odr:      cfg.odr,
	}

	// Setup our runner components list
	for t, f := range runner.factories {
		for _, n := range f.Registered() {
			runner.runner.Components = append(runner.runner.Components, &pb.Component{
				Type: pb.Component_Type(t),
				Name: n,
			})
		}
	}

	runner.logger.Debug("Created runner", "id", runner.id)

	return runner, nil
}

// Id returns the runner ID.
func (r *Runner) Id() string {
	return r.id
}

// Start starts the runner by registering the runner with the Waypoint
// server. This will spawn goroutines for management. This will return after
// registration so this should not be executed in a goroutine.
func (r *Runner) Start() error {
	if r.shutdown {
		return ErrClosed
	}

	log := r.logger

	// Register
	log.Debug("registering runner")
	client, err := r.client.RunnerConfig(r.runningCtx)
	if err != nil {
		return err
	}
	r.cleanup(func() { client.CloseSend() })

	// Send request
	if err := client.Send(&pb.RunnerConfigRequest{
		Event: &pb.RunnerConfigRequest_Open_{
			Open: &pb.RunnerConfigRequest_Open{
				Runner: r.runner,
			},
		},
	}); err != nil {
		return err
	}

	// Start the watcher and the gorotuine that receives configs
	ch := make(chan *pb.RunnerConfig)
	go r.watchConfig(r.runningCtx, ch)
	go r.recvConfig(r.runningCtx, client, ch)

	// Wait for the initial configuration to be set
	log.Debug("runner registered, waiting for first config processing")
	if r.waitState(&r.stateConfigOnce, true) {
		return status.Errorf(codes.Internal, "early exit while waiting for first config processing")
	}

	log.Info("runner registered with server and ready")
	return nil
}

// Close gracefully exits the runner. This will wait for any pending
// job executions to complete and then deregister the runner. After
// this is called, Start and Accept will no longer function and will
// return errors immediately.
func (r *Runner) Close() error {
	r.runningCond.L.Lock()
	defer r.runningCond.L.Unlock()

	// Wait for all the jobs to finish before we set the shutdown flag.
	for r.runningJobs > 0 {
		r.runningCond.Wait()
	}

	r.shutdown = true

	// Cancel the context that is used by Accept to wait on the RunnerJobStream.
	// This interrupts the Recv() call so that any goroutine running Accept()
	// knows that the Runner is closed now and can exit, avoiding a
	// goroutine leak.
	r.runningCancel()

	// Run any cleanup necessary
	if f := r.cleanupFunc; f != nil {
		f()
	}

	return nil
}

// waitState waits for the given state boolean to go true. This boolean
// must be a pointer to a state field on runner. This will also return if
// stateExit flips true. The return value notes whether we should exit.
func (r *Runner) waitState(state *bool, v bool) (exit bool) {
	r.stateCond.L.Lock()
	defer r.stateCond.L.Unlock()
	for *state != v && !r.stateExit {
		r.stateCond.Wait()
	}

	return r.stateExit
}

// setState sets the value of a state var on the runner struct and broadcasts
// the condition variable.
func (r *Runner) setState(state *bool, v bool) {
	r.stateCond.L.Lock()
	defer r.stateCond.L.Unlock()
	*state = v
	r.stateCond.Broadcast()
}

type config struct {
	byIdOnly bool
	odr      bool
}

type Option func(*Runner, *config) error

// WithClient sets the client directly. In this case, the runner won't
// attempt any connection at all regardless of other configuration (env
// vars or waypoint config file). This will be used.
func WithClient(client pb.WaypointClient) Option {
	return func(r *Runner, cfg *config) error {
		r.client = client
		return nil
	}
}

// WithComponentFactory sets a factory for a component type. If this isn't set for
// a component type, then the builtins will be used.
func WithComponentFactory(t component.Type, f *factory.Factory) Option {
	return func(r *Runner, cfg *config) error {
		r.factories[t] = f
		return nil
	}
}

// WithLogger sets the logger that the runner will use. If this isn't
// set it uses hclog.L().
func WithLogger(logger hclog.Logger) Option {
	return func(r *Runner, cfg *config) error {
		r.logger = logger
		return nil
	}
}

// WithLocal sets the runner to local mode. This only changes the UI
// behavior to use the given UI. If ui is nil then the normal streamed
// UI will be used.
func WithLocal(ui terminal.UI) Option {
	return func(r *Runner, cfg *config) error {
		r.local = true
		r.ui = ui
		return nil
	}
}

// ByIdOnly sets it so that only jobs that target this runner by specific
// ID may be assigned.
func ByIdOnly() Option {
	return func(r *Runner, cfg *config) error {
		cfg.byIdOnly = true
		return nil
	}
}

// WithODR configures this runner to be an on-demand runner. This
// will flag this to the server on registration.
func WithODR() Option {
	return func(r *Runner, cfg *config) error {
		cfg.odr = true
		return nil
	}
}

func WithDynamicConfig(set bool) Option {
	return func(r *Runner, cfg *config) error {
		r.enableDynConfig = set
		return nil
	}
}

// WithId sets the id of the runner directly. This isused when the when the server
// is expecting the runner to use a certain ID, such as when used via ondemand runners.
func WithId(id string) Option {
	return func(r *Runner, cfg *config) error {
		r.id = id
		return nil
	}
}

// WithAcceptTimeout sets a maximum amount of time to wait for a job before returning
// that one was not accepted.
func WithAcceptTimeout(dur time.Duration) Option {
	return func(r *Runner, cfg *config) error {
		r.acceptTimeout = dur
		return nil
	}
}
