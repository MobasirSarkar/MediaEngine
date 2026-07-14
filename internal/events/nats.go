package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	errs "github.com/MobasirSarkar/MediaEngine/internal/err"
)

type natsBus struct {
	conn   *nats.Conn
	js     jetstream.JetStream
	stream string
}

type Config struct {
	URL    string
	Stream string
}

func New(ctx context.Context, cfg Config) (Events, error) {
	if cfg.URL == "" {
		return nil, errs.New(errs.ErrInvalid, "nats url required", nil)
	}
	nc, err := nats.Connect(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("events: connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("events: jetstream: %w", err)
	}
	stream := cfg.Stream
	if stream == "" {
		stream = "pipeline"
	}
	return &natsBus{conn: nc, js: js, stream: stream}, nil
}

func (b *natsBus) Close(_ context.Context) error {
	b.conn.Close()
	return nil
}

func (b *natsBus) EnsureStream(ctx context.Context) error {
	_, err := b.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     b.stream,
		Subjects: []string{"pipeline.>"},
	})
	if err != nil {
		return fmt.Errorf("events: ensure stream: %w", err)
	}
	return nil
}

func (b *natsBus) Publish(ctx context.Context, subject Subject, payload []byte, headers map[string]string) error {
	msg := nats.NewMsg(subject)
	msg.Data = payload
	for k, v := range headers {
		msg.Header.Set(k, v)
	}
	if _, err := b.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("events: publish %s: %w", subject, err)
	}
	return nil
}

func (b *natsBus) Subscribe(ctx context.Context, subject Subject, durable string, handler Handler) error {
	dur := durable
	if dur == "" {
		dur = strings.ReplaceAll(subject, ".", "-")
	}
	cons, err := b.js.CreateOrUpdateConsumer(ctx, b.stream, jetstream.ConsumerConfig{
		Name:          dur,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		BackOff:       []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second},
	})
	if err != nil {
		return fmt.Errorf("events: consumer %s: %w", subject, err)
	}
	go b.consumeLoop(ctx, cons, handler)
	return nil
}

func (b *natsBus) consumeLoop(ctx context.Context, cons jetstream.Consumer, handler Handler) {
	for {
		msgs, err := cons.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, jetstream.ErrMsgIteratorClosed) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		for m := range msgs.Messages() {
			headers := map[string]string{}
			mh := m.Headers()
			for k, v := range mh {
				if len(v) > 0 {
					headers[k] = v[0]
				}
			}
			err := handler(ctx, Message{
				Subject: m.Subject(),
				Payload: append([]byte(nil), m.Data()...),
				Headers: headers,
				Ack:     func() error { return m.Ack() },
				Nak:     func() error { return m.Nak() },
			})
			if err != nil {
				_ = m.Nak()
				continue
			}
			_ = m.Ack()
		}
	}
}
