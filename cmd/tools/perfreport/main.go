// Command perfreport summarizes the perf.jsonl file Hive Desktop records in a
// dev build. Aggregation lives here, after the fact, because the file keeps
// every sample (ADR ui-performance-spans-are-recorded-to-jsonl).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"
)

func main() {
	command := &cli.Command{
		Name:      "perfreport",
		Usage:     "summarize a Hive Desktop perf.jsonl, optionally against a baseline",
		ArgsUsage: "<perf.jsonl>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "baseline", Usage: "an earlier perf.jsonl to compare against"},
			&cli.StringFlag{Name: "scope", Usage: "only spans in this scope (terminal, ui, bench)"},
			&cli.DurationFlag{Name: "since", Usage: "only spans newer than this, relative to the file's newest"},
		},
		Action: run,
	}
	if err := command.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "perfreport:", err)
		os.Exit(1)
	}
}

func run(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return errors.New("expected one perf.jsonl path")
	}
	filter := spanFilter{scope: cmd.String("scope"), since: cmd.Duration("since")}
	current, err := loadSpans(cmd.Args().First(), filter)
	if err != nil {
		return err
	}
	var baseline []span
	if path := cmd.String("baseline"); path != "" {
		if baseline, err = loadSpans(path, filter); err != nil {
			return err
		}
	}
	return writeReport(os.Stdout, current, baseline)
}

type span struct {
	At         time.Time      `json:"at"`
	Scope      string         `json:"scope"`
	Name       string         `json:"name"`
	DurationMs float64        `json:"durationMs"`
	Attrs      map[string]any `json:"attrs"`
}

// A benchmark span is one scenario, so its scenario belongs in the key or every
// workload lands in one row.
func (s span) key() string {
	key := s.Scope + " " + s.Name
	if scenario, ok := s.Attrs["scenario"].(string); ok {
		key += " " + scenario
	}
	return key
}

type spanFilter struct {
	scope string
	since time.Duration
}

func loadSpans(path string, filter spanFilter) ([]span, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return parseSpans(file, filter)
}

func parseSpans(r io.Reader, filter spanFilter) ([]span, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	var spans []span
	var newest time.Time
	for scanner.Scan() {
		var parsed span
		// The recorder may be mid-write on the last line of a live file.
		if err := json.Unmarshal(scanner.Bytes(), &parsed); err != nil {
			continue
		}
		if parsed.At.After(newest) {
			newest = parsed.At
		}
		if filter.scope == "" || parsed.Scope == filter.scope {
			spans = append(spans, parsed)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if filter.since > 0 {
		cutoff := newest.Add(-filter.since)
		spans = slices.DeleteFunc(spans, func(s span) bool { return s.At.Before(cutoff) })
	}
	return spans, nil
}

type summary struct {
	Key  string
	N    int
	P50  float64
	P95  float64
	Max  float64
	Long int
}

// longSpanMs is what counts as a visible hitch in the table's last column.
const longSpanMs = 50

func summarize(spans []span) []summary {
	groups := map[string][]float64{}
	for _, s := range spans {
		groups[s.key()] = append(groups[s.key()], s.DurationMs)
	}
	out := make([]summary, 0, len(groups))
	for key, durations := range groups {
		slices.Sort(durations)
		long := 0
		for _, d := range durations {
			if d >= longSpanMs {
				long++
			}
		}
		out = append(out, summary{
			Key:  key,
			N:    len(durations),
			P50:  percentile(durations, 50),
			P95:  percentile(durations, 95),
			Max:  durations[len(durations)-1],
			Long: long,
		})
	}
	slices.SortFunc(out, func(a, b summary) int { return strings.Compare(a.Key, b.Key) })
	return out
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[max(0, min(index, len(sorted)-1))]
}

// scenarioMetrics pulls the numeric results of benchmark scenarios, newest last
// wins: the same scenario run twice reports the later run.
func scenarioMetrics(spans []span) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	for _, s := range spans {
		scenario, ok := s.Attrs["scenario"].(string)
		if !ok || s.Name != "scenario" {
			continue
		}
		metrics := map[string]float64{}
		for name, value := range s.Attrs {
			if number, isNumber := value.(float64); isNumber {
				metrics[name] = number
			}
		}
		out[scenario] = metrics
	}
	return out
}

func writeReport(w io.Writer, current, baseline []span) error {
	table := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
	before := map[string]summary{}
	for _, s := range summarize(baseline) {
		before[s.Key] = s
	}
	if len(baseline) > 0 {
		sayf(table, "OPERATION\tN\tP50\tP95\tMAX\t>=50MS\tP95 BEFORE\tCHANGE\n")
	} else {
		sayf(table, "OPERATION\tN\tP50\tP95\tMAX\t>=50MS\n")
	}
	for _, s := range summarize(current) {
		sayf(table, "%s\t%d\t%.1f\t%.1f\t%.1f\t%d", s.Key, s.N, s.P50, s.P95, s.Max, s.Long)
		if was, ok := before[s.Key]; ok {
			sayf(table, "\t%.1f\t%s", was.P95, change(was.P95, s.P95))
		}
		sayf(table, "\n")
	}
	if err := table.Flush(); err != nil {
		return err
	}
	return writeScenarios(w, scenarioMetrics(current), scenarioMetrics(baseline))
}

func writeScenarios(w io.Writer, current, baseline map[string]map[string]float64) error {
	names := make([]string, 0, len(current))
	for name := range current {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		sayf(w, "\n%s\n", name)
		metrics := make([]string, 0, len(current[name]))
		for metric := range current[name] {
			metrics = append(metrics, metric)
		}
		slices.Sort(metrics)
		table := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
		for _, metric := range metrics {
			value := current[name][metric]
			if was, ok := baseline[name][metric]; ok {
				sayf(table, "  %s\t%g\twas %g\t%s\n", metric, value, was, change(was, value))
				continue
			}
			sayf(table, "  %s\t%g\n", metric, value)
		}
		if err := table.Flush(); err != nil {
			return err
		}
	}
	return nil
}

// sayf leaves write errors to the writer's Flush, which reports the first one.
func sayf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func change(was, now float64) string {
	if was == 0 {
		return ""
	}
	return fmt.Sprintf("%+.0f%%", (now-was)/was*100)
}
