package main

import (
	"errors"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/sbombastic/internal/messaging"
)

func main() {
	nc, err := nats.Connect("nats://controller-nats.sbombastic.svc.cluster.local")
	if err != nil {
		log.Fatal(err)
	}

	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		log.Fatal(err)
	}

	sub, err := js.PullSubscribe(messaging.SbombasticSubject, "worker", nats.InactiveThreshold(24*time.Hour))
	if err != nil {
		log.Fatal(err)
	}

	// TODO: placeholder, implement subscriber logic
	for {
		msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			log.Fatal(err)
		}

		for _, msg := range msgs {
			_ = msg.Ack()
		}
	}
}
