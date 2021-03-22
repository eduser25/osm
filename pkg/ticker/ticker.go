package ticker

import (
	"time"

	"github.com/openservicemesh/osm/pkg/announcements"
	"github.com/openservicemesh/osm/pkg/configurator"
	"github.com/openservicemesh/osm/pkg/kubernetes/events"
	"github.com/openservicemesh/osm/pkg/logger"
)

const (
	// Value used to identify ticker stop (0)
	stopTickerDuration = time.Duration(0)
	// Minimum allowed ticker duration
	minimumTickerDuration = time.Duration(1 * time.Minute)
)

// ResyncTicker contains the stop configuration for the ticker routines
type ResyncTicker struct {
	stopTickerRoutine chan struct{}
	stopConfigRoutine chan struct{}
}

var (
	log = logger.New("ticker")
	// Local reference to global ticker
	rTicker *ResyncTicker = nil
)

// InitTicker initializes a global ticker that is configured via
// pubsub, and triggers global proxy updates also through pubsub.
// Upon this function return, the ticker is guaranteed to be started
// and ready to receive new events.
func InitTicker(c configurator.Configurator) *ResyncTicker {
	if rTicker != nil {
		return rTicker
	}

	// Start config resync ticker routine
	doneInit := make(chan struct{})
	stopTicker := make(chan struct{})
	go ticker(doneInit, stopTicker)
	<-doneInit

	// Start config listener
	doneInit = make(chan struct{})
	stopConfig := make(chan struct{})
	go tickerConfigListener(c, doneInit, stopConfig)
	<-doneInit

	rTicker = &ResyncTicker{
		stopTickerRoutine: stopTicker,
		stopConfigRoutine: stopConfig,
	}
	return rTicker
}

// Returns if a given duration is valid
func validTickerDuration(d time.Duration) bool {
	return d >= minimumTickerDuration
}

// Listens to configmap events and notifies ticker routine to start/stop
func tickerConfigListener(c configurator.Configurator, ready chan struct{}, stop <-chan struct{}) {
	// Subscribe to configuration updates
	ch := events.GetPubSubInstance().Subscribe(
		announcements.ConfigMapAdded,
		announcements.ConfigMapDeleted,
		announcements.ConfigMapUpdated)

	// Run config listener
	// Bootstrap after subscribing
	prevDuration := c.GetConfigResyncInterval()

	// Initial config
	if validTickerDuration(prevDuration) {
		events.GetPubSubInstance().Publish(events.PubSubMessage{
			AnnouncementType: announcements.TickerStart,
			NewObj:           prevDuration,
		})
	}
	close(ready)

	for {
		select {
		case <-ch:
			duration := c.GetConfigResyncInterval()
			// Skip no changes from current applied conf
			if prevDuration == duration {
				continue
			}

			// We have a change
			if duration == stopTickerDuration {
				events.GetPubSubInstance().Publish(events.PubSubMessage{
					AnnouncementType: announcements.TickerStop,
				})
			} else {
				if validTickerDuration(duration) {
					// Notify to re/start ticker
					events.GetPubSubInstance().Publish(events.PubSubMessage{
						AnnouncementType: announcements.TickerStart,
						NewObj:           duration,
					})
				} else {
					log.Warn().Msgf("Invalid resync interval (%s < %s)", duration, minimumTickerDuration)
					continue
				}
			}

			prevDuration = duration
		case <-stop:
			return
		}
	}
}

func ticker(ready chan struct{}, stop <-chan struct{}) {
	ticker := make(<-chan time.Time)
	tickStart := events.GetPubSubInstance().Subscribe(
		announcements.TickerStart)
	tickStop := events.GetPubSubInstance().Subscribe(
		announcements.TickerStop)

	// Notify the calling function we are ready to receive events
	// Necessary as starting the ticker could loose events by the
	// caller if the caller intends to immedaitely start it
	close(ready)

	for {
		select {
		case msg := <-tickStart:
			psubMsg, ok := msg.(events.PubSubMessage)
			if !ok {
				log.Error().Msgf("Could not cast to pubsub msg %v", msg)
				continue
			}

			// Cast new object to duration value
			tickerDuration, ok := psubMsg.NewObj.(time.Duration)
			if !ok {
				log.Error().Msgf("Failed to cast ticker duration %v", psubMsg)
				continue
			}

			log.Info().Msgf("Ticker Starting with duration of %s", tickerDuration)
			ticker = time.NewTicker(tickerDuration).C
		case <-tickStop:
			log.Info().Msgf("Ticker Stopping")
			ticker = make(<-chan time.Time)
		case <-ticker:
			log.Info().Msgf("Ticker requesting broadcast proxy update")
			events.GetPubSubInstance().Publish(
				events.PubSubMessage{
					AnnouncementType: announcements.ScheduleProxyBroadcast,
				},
			)
		case <-stop:
			return
		}
	}
}
