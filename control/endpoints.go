package control

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/joyent/containerpilot/events"
)

// Endpoints wraps the EventBus so we can bridge data across the App and
// HTTPServer API boundary
type Endpoints struct {
	bus *events.EventBus
}

// PostHandler is an adapter which allows a normal function to serve itself and
// handle incoming HTTP POST requests, and allows us to pass thru EventBus to
// handlers
type PostHandler func(*http.Request) (interface{}, int)

func (pw PostHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		failedStatus := http.StatusNotImplemented
		http.Error(w, http.StatusText(failedStatus), failedStatus)
		return
	}
	resp, status := pw(r)
	switch status {
	case http.StatusOK:
		if resp != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		} else {
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "\n")
		}
	default:
		http.Error(w, http.StatusText(status), status)
	}
}

// PutEnviron handles incoming HTTP POST requests containing JSON environment
// variables and updates the environment of our current ContainerPilot
// process. Returns empty response or HTTP422.
func (e Endpoints) PutEnviron(r *http.Request) (interface{}, int) {
	var postEnv map[string]string
	jsonBlob, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		return nil, http.StatusUnprocessableEntity
	}
	err = json.Unmarshal(jsonBlob, &postEnv)
	if err != nil {
		return nil, http.StatusUnprocessableEntity
	}
	for envKey, envValue := range postEnv {
		os.Setenv(envKey, envValue)
	}
	return nil, http.StatusOK
}

// PostReload handles incoming HTTP POST requests and reloads our current
// ContainerPilot process configuration.  Returns empty response or HTTP422.
func (e Endpoints) PostReload(r *http.Request) (interface{}, int) {
	log.Debug("control: reloading app via control plane")
	defer r.Body.Close()
	e.bus.SetReloadFlag()
	e.bus.Shutdown()
	log.Debug("control: reloaded app via control plane")
	return nil, http.StatusOK
}

// PostMetric handles incoming HTTP POST requests, serializes the metrics
// into Events, and publishes them for sensors to record their values.
// Returns empty response or HTTP422.
func (e Endpoints) PostMetric(r *http.Request) (interface{}, int) {
	var postMetrics map[string]interface{}
	jsonBlob, err := ioutil.ReadAll(r.Body)

	defer r.Body.Close()
	if err != nil {
		return nil, http.StatusUnprocessableEntity
	}
	err = json.Unmarshal(jsonBlob, &postMetrics)
	if err != nil {
		log.Debug(err)
		return nil, http.StatusUnprocessableEntity
	}
	for metricKey, metricValue := range postMetrics {
		eventVal := fmt.Sprintf("%v|%v", metricKey, metricValue)
		e.bus.Publish(events.Event{events.Metric, eventVal})
	}
	return nil, http.StatusOK
}
