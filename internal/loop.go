package internal

import (
	"context"
	"log"

	"github.com/caedis/noreza/internal/input"
	"github.com/caedis/noreza/internal/mapping"
	"github.com/caedis/noreza/internal/output"
)

func RunEventLoop(ctx context.Context, reader *input.Reader, store *mapping.Store, writer *output.Writer, latProfiler *LatencyProfiler) {
	events := make(chan mapping.JoystickEvent, 128)
	go reader.Stream(events)

	// Start scroll wheel ticker for smooth continuous scrolling
	store.StartScrollTicker(ctx, func(keys []mapping.KeyMapping) {
		writer.Apply(keys, nil)
	})

	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-events:
			if !evt.Ready {
				log.Fatal("read error")
				return
			}

			store.BroadcastEvent(mapping.SSEEvent{Type: mapping.EventJoystick, Data: evt})
			press, release := store.Resolve(evt)
			writer.Apply(press, release)

			if latProfiler != nil && evt.Timestamp > 0 {
				latProfiler.Record(evt.Timestamp)
			}
		}
	}
}
