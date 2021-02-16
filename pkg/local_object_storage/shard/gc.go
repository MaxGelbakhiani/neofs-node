package shard

import (
	"context"
	"time"

	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	meta "github.com/nspcc-dev/neofs-node/pkg/local_object_storage/metabase"
	"github.com/nspcc-dev/neofs-node/pkg/util"
	"github.com/nspcc-dev/neofs-node/pkg/util/logger"
	"go.uber.org/zap"
)

// Event represents class of external events.
type Event interface {
	typ() eventType
}

type eventType int

const (
	_ eventType = iota
)

type eventHandler func(context.Context, Event)

type eventHandlers struct {
	cancelFunc context.CancelFunc

	handlers []eventHandler
}

type gc struct {
	*gcCfg

	workerPool util.WorkerPool

	remover func()

	mEventHandler map[eventType]*eventHandlers
}

type gcCfg struct {
	eventChanInit func() <-chan Event

	removerInterval time.Duration

	log *logger.Logger

	workerPoolInit func(int) util.WorkerPool
}

func defaultGCCfg() *gcCfg {
	ch := make(chan Event)
	close(ch)

	return &gcCfg{
		eventChanInit: func() <-chan Event {
			return ch
		},
		removerInterval: 10 * time.Second,
		log:             zap.L(),
		workerPoolInit: func(int) util.WorkerPool {
			return nil
		},
	}
}

func (gc *gc) init() {
	sz := 0

	for _, v := range gc.mEventHandler {
		sz += len(v.handlers)
	}

	if sz > 0 {
		gc.workerPool = gc.workerPoolInit(sz)
	}

	go gc.tickRemover()
	go gc.listenEvents()
}

func (gc *gc) listenEvents() {
	eventChan := gc.eventChanInit()

	for {
		event, ok := <-eventChan
		if !ok {
			gc.log.Warn("stop event listener by closed channel")
			return
		}

		v, ok := gc.mEventHandler[event.typ()]
		if !ok {
			continue
		}

		v.cancelFunc()

		var ctx context.Context
		ctx, v.cancelFunc = context.WithCancel(context.Background())

		for _, h := range v.handlers {
			err := gc.workerPool.Submit(func() {
				h(ctx, event)
			})
			if err != nil {
				gc.log.Warn("could not submit GC job to worker pool",
					zap.String("error", err.Error()),
				)
			}
		}
	}
}

func (gc *gc) tickRemover() {
	timer := time.NewTimer(gc.removerInterval)
	defer timer.Stop()

	for {
		<-timer.C
		gc.remover()
		timer.Reset(gc.removerInterval)
	}
}

// iterates over metabase graveyard and deletes objects
// with GC-marked graves.
func (s *Shard) removeGarbage() {
	buf := make([]*object.Address, 0, s.rmBatchSize)

	// iterate over metabase graveyard and accumulate
	// objects with GC mark
	err := s.metaBase.IterateOverGraveyard(func(g *meta.Grave) error {
		if g.WithGCMark() {
			buf = append(buf, g.Address())
		}

		return nil
	})
	if err != nil {
		s.log.Warn("iterator over metabase graveyard failed",
			zap.String("error", err.Error()),
		)

		return
	} else if len(buf) == 0 {
		return
	}

	// delete accumulated objects
	_, err = s.Delete(new(DeletePrm).
		WithAddresses(buf...),
	)
	if err != nil {
		s.log.Warn("could not delete the objects",
			zap.String("error", err.Error()),
		)

		return
	}
}
