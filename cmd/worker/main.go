package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
    nc, err := nats.Connect("nats://controller-nats.sbombastic.svc.cluster.local")
	if err != nil {
		log.Fatal(err)
	}

	// Use the JetStream context to produce and consumer messages
	// that have been persisted.
	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		log.Fatal(err)
	}

	sub, err := js.PullSubscribe("sbombastic", "worker", nats.InactiveThreshold(24*time.Hour))
	if err != nil {
		log.Fatal(err)
	}

	for {
		msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			log.Fatal(err)
		}

		for _, msg := range msgs {
			fmt.Printf("Received a message: %s\n", string(msg.Data))
			meta, _ := msg.Metadata()
			fmt.Printf("Stream Sequence  : %v\n", meta.Sequence.Stream)
			fmt.Printf("Consumer Sequence: %v\n", meta.Sequence.Consumer)
			msg.Ack()
		}
	}
}
