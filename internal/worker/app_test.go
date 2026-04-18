package worker

import (
	"fmt"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ackCall struct {
	tag      uint64
	multiple bool
}

type nackCall struct {
	tag      uint64
	multiple bool
	requeue  bool
}

type stubAcknowledger struct {
	ackCalls  []ackCall
	nackCalls []nackCall
}

func (a *stubAcknowledger) Ack(tag uint64, multiple bool) error {
	a.ackCalls = append(a.ackCalls, ackCall{tag: tag, multiple: multiple})
	return nil
}

func (a *stubAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	a.nackCalls = append(a.nackCalls, nackCall{tag: tag, multiple: multiple, requeue: requeue})
	return nil
}

func (a *stubAcknowledger) Reject(tag uint64, requeue bool) error {
	return nil
}

func TestRepublishOrRequeue_AcknowledgesOnSuccessfulPublish(t *testing.T) {
	acknowledger := &stubAcknowledger{}
	delivery := amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  42,
	}
	afterPublishCalled := false

	err := republishOrRequeue(delivery, func() error {
		return nil
	}, func() {
		afterPublishCalled = true
	})

	require.NoError(t, err)
	assert.True(t, afterPublishCalled)
	require.Len(t, acknowledger.ackCalls, 1)
	assert.Equal(t, ackCall{tag: 42, multiple: false}, acknowledger.ackCalls[0])
	assert.Empty(t, acknowledger.nackCalls)
}

func TestRepublishOrRequeue_RequeuesOriginalWhenPublishFails(t *testing.T) {
	acknowledger := &stubAcknowledger{}
	delivery := amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  42,
	}
	afterPublishCalled := false
	publishErr := fmt.Errorf("publish failed")

	err := republishOrRequeue(delivery, func() error {
		return publishErr
	}, func() {
		afterPublishCalled = true
	})

	require.ErrorIs(t, err, publishErr)
	assert.False(t, afterPublishCalled)
	assert.Empty(t, acknowledger.ackCalls)
	require.Len(t, acknowledger.nackCalls, 1)
	assert.Equal(t, nackCall{tag: 42, multiple: false, requeue: true}, acknowledger.nackCalls[0])
}
