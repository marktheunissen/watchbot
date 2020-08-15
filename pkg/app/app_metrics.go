package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	metrics "github.com/rcrowley/go-metrics"
)

type CamMetrics struct {
	snapshot       metrics.Counter
	uploadError    metrics.Counter
	uploadSuccess  metrics.Counter
	overviewDrop   metrics.Counter
	overviewSend   metrics.Counter
	boxSend        metrics.Counter
	boxDrop        metrics.Counter
	boxReject      metrics.Counter
	detectorError  metrics.Counter
	detectorNone   metrics.Counter
	detectorHit    metrics.Counter
	boxWidths      *HistVals
	boxHeights     *HistVals
	boxConfidences *HistVals
}

func NewCamMetrics(name string) *CamMetrics {
	name = strings.ToLower(name)
	return &CamMetrics{
		snapshot:       metrics.GetOrRegisterCounter(name+".snapshot", metrics.DefaultRegistry),
		uploadError:    metrics.GetOrRegisterCounter(name+".upload.error", metrics.DefaultRegistry),
		uploadSuccess:  metrics.GetOrRegisterCounter(name+".upload.success", metrics.DefaultRegistry),
		overviewDrop:   metrics.GetOrRegisterCounter(name+".overview.drop", metrics.DefaultRegistry),
		overviewSend:   metrics.GetOrRegisterCounter(name+".overview.send", metrics.DefaultRegistry),
		boxDrop:        metrics.GetOrRegisterCounter(name+".box.drop", metrics.DefaultRegistry),
		boxSend:        metrics.GetOrRegisterCounter(name+".box.send", metrics.DefaultRegistry),
		boxReject:      metrics.GetOrRegisterCounter(name+".box.reject", metrics.DefaultRegistry),
		detectorError:  metrics.GetOrRegisterCounter(name+".detector.error", metrics.DefaultRegistry),
		detectorNone:   metrics.GetOrRegisterCounter(name+".detector.none", metrics.DefaultRegistry),
		detectorHit:    metrics.GetOrRegisterCounter(name+".detector.hit", metrics.DefaultRegistry),
		boxWidths:      NewHistVals(name+" Box Widths", 40),
		boxHeights:     NewHistVals(name+" Box Heights", 40),
		boxConfidences: NewHistVals(name+" Confidences", 20),
	}
}

type appMetrics struct {
	mainTicker     metrics.Meter
	frameRead      metrics.Meter
	frameSkip      metrics.Meter
	supervisorTick metrics.Counter
	heartbeatTick  metrics.Counter
	heartbeatError metrics.Counter
	cams           []*CamMetrics
}

var stats = &appMetrics{
	mainTicker:     metrics.GetOrRegisterMeter("frame.total", metrics.DefaultRegistry),
	frameRead:      metrics.GetOrRegisterMeter("frame.read", metrics.DefaultRegistry),
	frameSkip:      metrics.GetOrRegisterMeter("frame.skip", metrics.DefaultRegistry),
	supervisorTick: metrics.GetOrRegisterCounter("supervisor.tick", metrics.DefaultRegistry),
	heartbeatTick:  metrics.GetOrRegisterCounter("heartbeat.tick", metrics.DefaultRegistry),
	heartbeatError: metrics.GetOrRegisterCounter("heartbeat.error", metrics.DefaultRegistry),
	cams:           []*CamMetrics{},
}

func MetricsPrintOut() string {
	r := metrics.DefaultRegistry
	scale := time.Millisecond
	du := float64(scale)
	duSuffix := scale.String()[1:]

	type m struct {
		Name string
		Repr string
	}
	allM := []m{}
	f := func(name string, i interface{}) {
		out := ""
		switch metric := i.(type) {
		case metrics.Counter:
			out += fmt.Sprintf("%s\n", name)
			out += fmt.Sprintf("  count:       %9d\n", metric.Count())
		case metrics.Gauge:
			out += fmt.Sprintf("%s\n", name)
			out += fmt.Sprintf("  value:       %9d\n", metric.Value())
		case metrics.GaugeFloat64:
			out += fmt.Sprintf("%s\n", name)
			out += fmt.Sprintf("  value:       %f\n", metric.Value())
		case metrics.Histogram:
			h := metric.Snapshot()
			ps := h.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
			out += fmt.Sprintf("%s\n", name)
			out += fmt.Sprintf("  count:       %9d\n", h.Count())
			out += fmt.Sprintf("  min:         %9d\n", h.Min())
			out += fmt.Sprintf("  max:         %9d\n", h.Max())
			out += fmt.Sprintf("  mean:        %12.2f\n", h.Mean())
			out += fmt.Sprintf("  stddev:      %12.2f\n", h.StdDev())
			out += fmt.Sprintf("  median:      %12.2f\n", ps[0])
			out += fmt.Sprintf("  75%%:         %12.2f\n", ps[1])
			out += fmt.Sprintf("  95%%:         %12.2f\n", ps[2])
			out += fmt.Sprintf("  99%%:         %12.2f\n", ps[3])
			out += fmt.Sprintf("  99.9%%:       %12.2f\n", ps[4])
		case metrics.Meter:
			m := metric.Snapshot()
			out += fmt.Sprintf("%s\n", name)
			out += fmt.Sprintf("  count:       %9d\n", m.Count())
			out += fmt.Sprintf("  1-min rate:  %12.2f\n", m.Rate1())
			out += fmt.Sprintf("  5-min rate:  %12.2f\n", m.Rate5())
			out += fmt.Sprintf("  15-min rate: %12.2f\n", m.Rate15())
			out += fmt.Sprintf("  mean rate:   %12.2f\n", m.RateMean())
		case metrics.Timer:
			t := metric.Snapshot()
			ps := t.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
			out += fmt.Sprintf("timer %s\n", name)
			out += fmt.Sprintf("  count:       %9d\n", t.Count())
			out += fmt.Sprintf("  min:         %12.2f%s\n", float64(t.Min())/du, duSuffix)
			out += fmt.Sprintf("  max:         %12.2f%s\n", float64(t.Max())/du, duSuffix)
			out += fmt.Sprintf("  mean:        %12.2f%s\n", t.Mean()/du, duSuffix)
			out += fmt.Sprintf("  stddev:      %12.2f%s\n", t.StdDev()/du, duSuffix)
			out += fmt.Sprintf("  median:      %12.2f%s\n", ps[0]/du, duSuffix)
			out += fmt.Sprintf("  75%%:         %12.2f%s\n", ps[1]/du, duSuffix)
			out += fmt.Sprintf("  95%%:         %12.2f%s\n", ps[2]/du, duSuffix)
			out += fmt.Sprintf("  99%%:         %12.2f%s\n", ps[3]/du, duSuffix)
			out += fmt.Sprintf("  99.9%%:       %12.2f%s\n", ps[4]/du, duSuffix)
			out += fmt.Sprintf("  1-min rate:  %12.2f\n", t.Rate1())
			out += fmt.Sprintf("  5-min rate:  %12.2f\n", t.Rate5())
			out += fmt.Sprintf("  15-min rate: %12.2f\n", t.Rate15())
			out += fmt.Sprintf("  mean rate:   %12.2f\n", t.RateMean())
		}
		allM = append(allM, m{Name: name, Repr: out})
	}
	r.Each(f)
	sort.SliceStable(allM, func(i, j int) bool { return allM[i].Name < allM[j].Name })
	out := "```\n"
	for _, val := range allM {
		out += val.Repr
	}
	out += "```\n"
	return out
}
