package kafkaconsumer

import (
	"fmt"
	"github.com/Shopify/sarama"
	"time"
)

// EventStream is an abstraction of a sarama.Consumer
type EventStream interface {
	Events() <-chan *sarama.ConsumerEvent
	Close() error
}

// EventBatch is a batch of events from a single topic/partition
type EventBatch struct {
	Topic     string
	Partition int32
	Events    []sarama.ConsumerEvent
}

// Returns true if starts with an OffsetOutOfRange error
func (b *EventBatch) offsetIsOutOfRange() bool {
	if b == nil || len(b.Events) < 1 {
		return false
	}

	err := b.Events[0].Err
	if err == nil {
		return false
	}

	kerr, ok := err.(sarama.KError)
	return ok && kerr == sarama.OffsetOutOfRange
}

// PartitionConsumer can consume a single partition of a single topic
type PartitionConsumer struct {
	stream    EventStream
	topic     string
	partition int32
	offset    int64
}

// NewPartitionConsumer creates a new partition consumer instance
func NewPartitionConsumer(group *ConsumerGroup, partition int32) (*PartitionConsumer, error) {
	config := sarama.ConsumerConfig{
		DefaultFetchSize: group.config.DefaultFetchSize,
		EventBufferSize:  group.config.EventBufferSize,
		MaxMessageSize:   group.config.MaxMessageSize,
		MaxWaitTime:      group.config.MaxWaitTime,
		MinFetchSize:     group.config.MinFetchSize,
		OffsetMethod:     sarama.OffsetMethodOldest,
	}

	offset, err := group.Offset(partition)
	if err != nil {
		return nil, err
	} else if offset > 0 {
		config.OffsetMethod = sarama.OffsetMethodManual
		config.OffsetValue = offset + 1
	}

	stream, err := sarama.NewConsumer(group.client, group.topic, partition, group.name, &config)
	if err != nil {
		return nil, err
	}

	return &PartitionConsumer{
		stream:    stream,
		topic:     group.topic,
		partition: partition,
	}, nil
}

// Fetch returns a batch of events
// WARNING: may return nil if not events are available
func (p *PartitionConsumer) Fetch(stream chan *Event, duration time.Duration) error {
	events := p.stream.Events()
	timeout := time.After(duration)

	for {
		select {
		case <-timeout:
			return nil
		case event, ok := <-events:
			if !ok {
				return fmt.Errorf("events channel was closed")
			}
			if event.Err != nil {
				return event.Err
			}
			stream <- &Event{ConsumerEvent: *event, Topic: p.topic, Partition: p.partition}
			if event.Err == nil && event.Offset > p.offset {
				p.offset = event.Offset
			}
		}
	}
}

// Close closes a partition consumer
func (p *PartitionConsumer) Close() error {
	return p.stream.Close()
}
