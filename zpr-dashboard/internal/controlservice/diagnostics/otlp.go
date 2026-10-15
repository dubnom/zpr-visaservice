package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

type OTLPFiles map[string]string

func (sources OTLPFiles) Query(_ context.Context, source Source, _ int) ([]Log, []Metric, error) {
	path := sources[source.Identity]
	if path == "" {
		return nil, nil, fmt.Errorf("no operator OTLP file for node %s", source.Identity)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("node OTLP metrics file unavailable")
	}
	defer file.Close()
	var payload struct {
		ResourceMetrics []struct {
			Resource struct {
				Attributes []struct {
					Key   string `json:"key"`
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
				} `json:"attributes"`
			} `json:"resource"`
			ScopeMetrics []struct {
				Metrics []struct {
					Name  string `json:"name"`
					Gauge struct {
						DataPoints []otlpPoint `json:"dataPoints"`
					} `json:"gauge"`
					Sum struct {
						DataPoints []otlpPoint `json:"dataPoints"`
					} `json:"sum"`
				} `json:"metrics"`
			} `json:"scopeMetrics"`
		} `json:"resourceMetrics"`
	}
	content, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil || len(content) > 65536 || json.Unmarshal(content, &payload) != nil {
		return nil, nil, fmt.Errorf("invalid or oversized node OTLP metrics")
	}
	var metrics []Metric
	for _, resource := range payload.ResourceMetrics {
		identity := map[string]string{}
		for _, attribute := range resource.Resource.Attributes {
			identity[attribute.Key] = attribute.Value.StringValue
		}
		if identity["service.name"] != source.ServiceName || identity["service.instance.id"] != source.InstanceID {
			continue
		}
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, point := range append(metric.Gauge.DataPoints, metric.Sum.DataPoints...) {
					nanos, err := strconv.ParseInt(point.TimeUnixNano, 10, 64)
					if err != nil {
						return nil, nil, fmt.Errorf("invalid node metric timestamp")
					}
					metrics = append(metrics, Metric{Name: metric.Name, Value: point.AsInt, Timestamp: time.Unix(0, nanos)})
				}
			}
		}
	}
	if len(metrics) == 0 {
		return nil, nil, fmt.Errorf("node OTLP resource identity or metrics missing")
	}
	return nil, metrics, nil
}

type otlpPoint struct {
	TimeUnixNano string `json:"timeUnixNano"`
	AsInt        string `json:"asInt"`
}

var (
	_ Provider = (*OpenObserve)(nil)
	_ Provider = OTLPFiles(nil)
)
