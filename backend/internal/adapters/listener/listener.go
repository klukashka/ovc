package listener

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const ApplicationName = "ovc_realtime_listener"

type Generation uint64

type LossKind string

const (
	LossConnect    LossKind = "connect"
	LossListen     LossKind = "listen"
	LossConnection LossKind = "connection"
	LossHeartbeat  LossKind = "heartbeat"
	LossPanic      LossKind = "panic"
)

type Heartbeats interface {
	Heartbeat(ctx context.Context, connection *pgx.Conn, instance domain.InstanceID) error
}

type ConnectionHeartbeats struct{}

const notifyStatement = "SELECT pg_notify($1, $2)"

func (ConnectionHeartbeats) Heartbeat(ctx context.Context, connection *pgx.Conn, instance domain.InstanceID) error {
	payload, err := domain.EncodeSignal(domain.HeartbeatSignal{Instance: instance})
	if err != nil {
		return fmt.Errorf("encode the heartbeat: %w", err)
	}
	if _, err := connection.Exec(ctx, notifyStatement, domain.SignalChannel, string(payload)); err != nil {
		return fmt.Errorf("notify the heartbeat: %w", err)
	}
	return nil
}

type Consumer interface {
	ListenerConnected(generation Generation)
	ListenerLost(generation Generation, kind LossKind)
	Signal(signal domain.Signal)
}

type Timing struct {
	HeartbeatInterval time.Duration
	LossAfter         time.Duration
	MinBackoff        time.Duration
	MaxBackoff        time.Duration
	ConnectTimeout    time.Duration
	CloseTimeout      time.Duration
}

func DefaultTiming() Timing {
	return Timing{
		HeartbeatInterval: 2 * time.Second,
		LossAfter:         5 * time.Second,
		MinBackoff:        time.Second,
		MaxBackoff:        30 * time.Second,
		ConnectTimeout:    5 * time.Second,
		CloseTimeout:      time.Second,
	}
}

type Dependencies struct {
	DatabaseURL core.Secret
	Instance    domain.InstanceID
	Heartbeats  Heartbeats
	Consumer    Consumer
	Logger      *slog.Logger
	Timing      Timing
}

type Listener struct {
	config     *pgx.ConnConfig
	instance   domain.InstanceID
	heartbeats Heartbeats
	consumer   Consumer
	logger     *slog.Logger
	timing     Timing
	state      atomic.Uint64
}

var (
	ErrInvalidDatabaseURL  = errors.New("the listener's database URL is not a valid PostgreSQL connection string")
	ErrInvalidTiming       = errors.New("the listener's timing is not usable")
	ErrInvalidInstance     = errors.New("the listener needs a random instance identifier")
	ErrMissingDependencies = errors.New("the listener needs its heartbeats, consumer and logger")

	errHeartbeatMissing = errors.New("no heartbeat of this instance arrived in time")
	errConsumerPanicked = errors.New("the listener's consumer panicked")
)

const (
	keepAliveIdle     = 5 * time.Second
	keepAliveInterval = 2 * time.Second
	keepAliveCount    = 3
	tcpUserTimeout    = 10 * time.Second
)

func New(dependencies Dependencies) (*Listener, error) {
	timing := dependencies.Timing
	if timing.HeartbeatInterval <= 0 || timing.LossAfter <= timing.HeartbeatInterval || timing.MinBackoff <= 0 ||
		timing.MaxBackoff < timing.MinBackoff || timing.ConnectTimeout <= 0 || timing.CloseTimeout <= 0 {
		return nil, ErrInvalidTiming
	}
	if dependencies.Instance == (domain.InstanceID{}) {
		return nil, ErrInvalidInstance
	}
	if dependencies.Heartbeats == nil || dependencies.Consumer == nil || dependencies.Logger == nil {
		return nil, ErrMissingDependencies
	}
	config, err := pgx.ParseConfig(dependencies.DatabaseURL.Reveal())
	if err != nil {
		return nil, ErrInvalidDatabaseURL
	}
	config.ConnectTimeout = timing.ConnectTimeout
	config.RuntimeParams["application_name"] = ApplicationName
	dialer := &net.Dialer{
		Timeout: timing.ConnectTimeout,
		KeepAliveConfig: net.KeepAliveConfig{
			Enable: true, Idle: keepAliveIdle, Interval: keepAliveInterval, Count: keepAliveCount,
		},
		Control: socketControl(tcpUserTimeout),
	}
	config.DialFunc = dialer.DialContext
	return &Listener{
		config: config, instance: dependencies.Instance, heartbeats: dependencies.Heartbeats,
		consumer: dependencies.Consumer, logger: dependencies.Logger, timing: timing,
	}, nil
}

const liveBit = 1

func (l *Listener) State() (Generation, bool) {
	state := l.state.Load()
	return Generation(state >> 1), state&liveBit == liveBit
}

func (l *Listener) becomeLive() Generation {
	for {
		current := l.state.Load()
		next := (current>>1+1)<<1 | liveBit
		if l.state.CompareAndSwap(current, next) {
			return Generation(next >> 1)
		}
	}
}

func (l *Listener) becomeLost(generation Generation) {
	live := uint64(generation)<<1 | liveBit
	l.state.CompareAndSwap(live, live&^liveBit)
}

func (l *Listener) Run(ctx context.Context) {
	backoff := l.timing.MinBackoff
	for ctx.Err() == nil {
		if l.attempt(ctx) {
			backoff = l.timing.MinBackoff
		}
		if !l.wait(ctx, backoff) {
			return
		}
		backoff = min(2*backoff, l.timing.MaxBackoff)
	}
}

func (l *Listener) attempt(ctx context.Context) bool {
	proved := make(chan bool, 1)
	err := <-core.Run(ctx, l.logger, "realtime.listener.serve", func() error {
		proved <- l.connectAndServe(ctx)
		return nil
	})
	if err != nil {
		return false
	}
	return <-proved
}

func (l *Listener) wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type loss struct {
	listener   *Listener
	ctx        context.Context
	generation Generation
	once       sync.Once
	mu         sync.Mutex
	sendErr    error
}

func (r *loss) heartbeatFailed(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sendErr = err
}

func (r *loss) heartbeatSent() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sendErr = nil
}

func (r *loss) silence() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(errHeartbeatMissing, r.sendErr)
}

func (r *loss) report(kind LossKind, err error) {
	r.once.Do(func() {
		r.listener.becomeLost(r.generation)
		if r.ctx.Err() != nil {
			return
		}
		r.listener.logLost(r.ctx, r.generation, kind, err)
		r.listener.consumer.ListenerLost(r.generation, kind)
	})
}

func (l *Listener) connectAndServe(ctx context.Context) bool {
	connection, kind, err := l.connect(ctx)
	if err != nil {
		if ctx.Err() == nil {
			generation, _ := l.State()
			l.logLost(ctx, generation, kind, err)
		}
		return false
	}
	generation := l.becomeLive()
	lost := &loss{listener: l, ctx: ctx, generation: generation}
	served := false
	defer func() {
		if !served {
			lost.report(LossPanic, errConsumerPanicked)
		}
		l.closeDropped(ctx, connection)
	}()
	kind, proved, err := l.serve(ctx, connection, lost)
	served = true
	lost.report(kind, err)
	return proved
}

func (l *Listener) connect(ctx context.Context) (*pgx.Conn, LossKind, error) {
	connectContext, cancel := context.WithTimeout(ctx, l.timing.ConnectTimeout)
	defer cancel()
	connection, err := pgx.ConnectConfig(connectContext, l.config)
	if err != nil {
		return nil, LossConnect, fmt.Errorf("connect the listener: %w", err)
	}
	if _, err := connection.Exec(connectContext, "LISTEN "+pgx.Identifier{domain.SignalChannel}.Sanitize()); err != nil {
		closeErr := l.close(connection)
		return nil, LossListen, errors.Join(fmt.Errorf("listen: %w", err), closeErr)
	}
	return connection, "", nil
}

func (l *Listener) close(connection *pgx.Conn) error {
	closeContext, cancel := context.WithTimeout(context.Background(), l.timing.CloseTimeout)
	defer cancel()
	if err := connection.Close(closeContext); err != nil {
		return fmt.Errorf("close the listener connection: %w", err)
	}
	return nil
}

func (l *Listener) closeDropped(ctx context.Context, connection *pgx.Conn) {
	if err := l.close(connection); err != nil && ctx.Err() == nil {
		l.logger.LogAttrs(ctx, slog.LevelWarn, "realtime.listener.close_failed", slog.String("error_kind", core.ErrorKind(err)))
	}
}

func (l *Listener) logLost(ctx context.Context, generation Generation, kind LossKind, err error) {
	l.logger.LogAttrs(ctx, slog.LevelError, "realtime.listener.lost",
		slog.Uint64("generation", uint64(generation)),
		slog.String("kind", string(kind)),
		slog.String("error_kind", core.ErrorKind(err)),
	)
}

func (l *Listener) serve(ctx context.Context, connection *pgx.Conn, lost *loss) (LossKind, bool, error) {
	waiting, stop := context.WithCancelCause(ctx)
	silence := time.AfterFunc(l.timing.LossAfter, func() {
		core.Go(ctx, l.logger, "realtime.listener.silence", func(context.Context) {
			cause := lost.silence()
			stop(cause)
			lost.report(LossHeartbeat, cause)
		})
	})
	defer func() {
		silence.Stop()
		stop(nil)
		l.becomeLost(lost.generation)
	}()
	l.logger.LogAttrs(ctx, slog.LevelInfo, "realtime.listener.connected", slog.Uint64("generation", uint64(lost.generation)))
	l.consumer.ListenerConnected(lost.generation)
	proved := false
	received := func(notification *pgconn.Notification) {
		if l.handle(ctx, notification) {
			proved = true
			silence.Reset(l.timing.LossAfter)
		}
	}
	for {
		if err := <-core.Run(waiting, l.logger, "realtime.listener.heartbeat.send", func() error {
			l.sendHeartbeat(waiting, connection, lost)
			return nil
		}); err != nil {
			stop(err)
			return LossPanic, proved, err
		}
		if err := l.receiveUntilBeat(waiting, connection, received); err != nil {
			cause := context.Cause(waiting)
			switch {
			case errors.Is(cause, errHeartbeatMissing):
				return LossHeartbeat, proved, cause
			case errors.Is(cause, core.ErrPanicked):
				return LossPanic, proved, cause
			}
			return LossConnection, proved, err
		}
	}
}

func (l *Listener) receiveUntilBeat(ctx context.Context, connection *pgx.Conn, received func(*pgconn.Notification)) error {
	beat, cancel := context.WithTimeout(ctx, l.timing.HeartbeatInterval)
	defer cancel()
	for {
		notification, err := connection.WaitForNotification(beat)
		if notification != nil {
			received(notification)
		}
		switch {
		case err == nil:
		case ctx.Err() == nil && beat.Err() != nil && !connection.IsClosed():
			return nil
		default:
			return err
		}
	}
}

func (l *Listener) sendHeartbeat(ctx context.Context, connection *pgx.Conn, lost *loss) {
	if err := l.heartbeats.Heartbeat(ctx, connection, l.instance); err != nil {
		lost.heartbeatFailed(fmt.Errorf("send heartbeat: %w", err))
		return
	}
	lost.heartbeatSent()
}

func (l *Listener) handle(ctx context.Context, notification *pgconn.Notification) bool {
	signal, err := domain.ParseSignal(notification.Payload)
	if err != nil {
		l.logRejected(ctx, err, len(notification.Payload))
		return false
	}
	if heartbeat, ok := signal.(domain.HeartbeatSignal); ok {
		return heartbeat.Instance == l.instance
	}
	l.consumer.Signal(signal)
	return false
}

func (l *Listener) logRejected(ctx context.Context, err error, size int) {
	problem := "unknown"
	var signalError *domain.SignalError
	if errors.As(err, &signalError) {
		problem = string(signalError.Problem)
	}
	l.logger.LogAttrs(ctx, slog.LevelWarn, "realtime.signal.rejected", slog.String("reason", problem), slog.Int("bytes", size))
}
