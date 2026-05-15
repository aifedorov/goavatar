package telemetry

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

type AMQPHeadersCarrier amqp.Table

func (c AMQPHeadersCarrier) Get(key string) string {
	if v, ok := c[key].(string); ok {
		return v
	}
	return ""
}

func (c AMQPHeadersCarrier) Set(key, value string) {
	c[key] = value
}

func (c AMQPHeadersCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
