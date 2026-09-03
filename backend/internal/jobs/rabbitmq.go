package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

type BrokerConfig struct {
	URL          string
	Exchange     string
	ExtractQueue string
	ResultQueue  string
	// Logger receives reconnect diagnostics. Optional.
	Logger *slog.Logger
}

// publishTimeout bounds a single publish so an HTTP handler is never blocked
// indefinitely by broker flow control or a half-open connection.
const publishTimeout = 5 * time.Second

type Broker struct {
	config BrokerConfig
	logger *slog.Logger

	mu         sync.RWMutex
	connection *amqp091.Connection
	channel    *amqp091.Channel

	closeOnce sync.Once
	done      chan struct{}
}

func OpenBroker(config BrokerConfig) (*Broker, error) {
	if config.URL == "" {
		return nil, errors.New("RabbitMQ URL is not configured")
	}
	if config.Exchange == "" {
		config.Exchange = ExchangeName
	}
	if config.ExtractQueue == "" {
		config.ExtractQueue = "sta.admissions.extract"
	}
	if config.ResultQueue == "" {
		config.ResultQueue = "sta.admissions.extracted"
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	broker := &Broker{config: config, logger: logger, done: make(chan struct{})}
	if err := broker.connect(); err != nil {
		return nil, err
	}
	go broker.superviseConnection()
	return broker, nil
}

// connect dials the broker, opens a channel and (re)declares the topology,
// then swaps them in under the write lock.
func (b *Broker) connect() error {
	connection, err := amqp091.Dial(b.config.URL)
	if err != nil {
		return fmt.Errorf("connect RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		connection.Close()
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if err := declareTopology(channel, b.config); err != nil {
		connection.Close()
		return err
	}
	b.mu.Lock()
	b.connection = connection
	b.channel = channel
	b.mu.Unlock()
	return nil
}

// superviseConnection redials with capped exponential backoff whenever the
// current connection closes, until Close is called.
func (b *Broker) superviseConnection() {
	for {
		b.mu.RLock()
		connection := b.connection
		b.mu.RUnlock()
		if connection == nil {
			return
		}
		closed := connection.NotifyClose(make(chan *amqp091.Error, 1))
		select {
		case <-b.done:
			return
		case reason, ok := <-closed:
			if !ok {
				// Channel closed without an error: a deliberate shutdown.
				return
			}
			b.logger.Warn("RabbitMQ connection lost; reconnecting", "reason", reason)
		}

		backoff := 500 * time.Millisecond
		for {
			select {
			case <-b.done:
				return
			case <-time.After(backoff):
			}
			if err := b.connect(); err != nil {
				b.logger.Warn("RabbitMQ reconnect failed", "error", err, "retry_in", backoff)
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
			b.logger.Info("RabbitMQ reconnected")
			break
		}
	}
}

func declareTopology(channel *amqp091.Channel, config BrokerConfig) error {
	if err := channel.ExchangeDeclare(config.Exchange, amqp091.ExchangeTopic, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare event exchange: %w", err)
	}
	deadExchange := config.Exchange + DeadLetterExchangeSuffix
	if err := channel.ExchangeDeclare(deadExchange, amqp091.ExchangeDirect, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange: %w", err)
	}
	if err := declareQueue(channel, config.Exchange, config.ExtractQueue, deadExchange, ExtractRoutingKey, CandidateListExtractRoutingKey); err != nil {
		return err
	}
	return declareQueue(channel, config.Exchange, config.ResultQueue, deadExchange, ExtractedRoutingKey, CandidateListExtractedRoutingKey)
}

func declareQueue(channel *amqp091.Channel, exchange, name, deadExchange string, routingKeys ...string) error {
	deadQueue := name + ".dead"
	if _, err := channel.QueueDeclare(deadQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead queue %s: %w", deadQueue, err)
	}
	if err := channel.QueueBind(deadQueue, name, deadExchange, false, nil); err != nil {
		return fmt.Errorf("bind dead queue %s: %w", deadQueue, err)
	}
	args := amqp091.Table{
		"x-dead-letter-exchange":    deadExchange,
		"x-dead-letter-routing-key": name,
	}
	if _, err := channel.QueueDeclare(name, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare queue %s: %w", name, err)
	}
	for _, routingKey := range routingKeys {
		if err := channel.QueueBind(name, routingKey, exchange, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", name, err)
		}
	}
	return nil
}

// Publish sends a durable JSON message. It bounds the attempt with a short
// timeout and, if the channel is closed, waits briefly for the supervisor to
// reconnect and retries once.
func (b *Broker) Publish(ctx context.Context, routingKey string, messageID uuid.UUID, payload any) error {
	if b == nil {
		return errors.New("RabbitMQ broker is not ready")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal job message: %w", err)
	}
	publishing := amqp091.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		MessageId:    messageID.String(),
		Timestamp:    time.Now().UTC(),
		Body:         body,
	}

	err = b.publishOnce(ctx, routingKey, publishing)
	if err == nil || !errors.Is(err, amqp091.ErrClosed) {
		return err
	}
	// Give the supervisor a moment to swap in a fresh channel, then retry once.
	select {
	case <-time.After(time.Second):
	case <-ctx.Done():
		return ctx.Err()
	case <-b.done:
		return errors.New("RabbitMQ broker is closing")
	}
	return b.publishOnce(ctx, routingKey, publishing)
}

func (b *Broker) publishOnce(ctx context.Context, routingKey string, publishing amqp091.Publishing) error {
	b.mu.RLock()
	channel := b.channel
	b.mu.RUnlock()
	if channel == nil {
		return errors.New("RabbitMQ broker is not ready")
	}
	attemptCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	return channel.PublishWithContext(attemptCtx, b.config.Exchange, routingKey, false, false, publishing)
}

func (b *Broker) Consume(queue string, consumer string) (<-chan amqp091.Delivery, error) {
	b.mu.RLock()
	channel := b.channel
	b.mu.RUnlock()
	if channel == nil {
		return nil, errors.New("RabbitMQ broker is not ready")
	}
	if err := channel.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("configure RabbitMQ QoS: %w", err)
	}
	deliveries, err := channel.Consume(queue, consumer, false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("consume RabbitMQ queue: %w", err)
	}
	return deliveries, nil
}

// Ping reports whether the broker connection is still usable. It is cheap and
// suitable for a readiness probe.
func (b *Broker) Ping(context.Context) error {
	if b == nil {
		return errors.New("RabbitMQ broker is not ready")
	}
	b.mu.RLock()
	connection := b.connection
	b.mu.RUnlock()
	if connection == nil {
		return errors.New("RabbitMQ broker is not ready")
	}
	if connection.IsClosed() {
		return errors.New("RabbitMQ connection is closed")
	}
	return nil
}

func (b *Broker) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() { close(b.done) })
	b.mu.Lock()
	channel, connection := b.channel, b.connection
	b.channel, b.connection = nil, nil
	b.mu.Unlock()
	if channel != nil {
		_ = channel.Close()
	}
	if connection != nil {
		_ = connection.Close()
	}
}
