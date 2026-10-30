package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sample = `{"at":"2026-10-30T15:00:00Z","scope":"terminal","name":"input:echo","durationMs":10}
{"at":"2026-10-30T15:00:01Z","scope":"terminal","name":"input:echo","durationMs":30}
{"at":"2026-10-30T15:00:02Z","scope":"terminal","name":"input:echo","durationMs":60}
{"at":"2026-10-30T15:00:03Z","scope":"ui","name":"window","durationMs":10000}
{"at":"2026-10-30T15:00:04Z","scope":"bench","name":"scenario","durationMs":500,"attrs":{"scenario":"wall","mbPerSec":40,"webgl":true}}
{"at":"2026-10-30T15:00:04Z","scope":"terminal","name":"input:ec`

func TestParseSpansSkipsAPartialLastLine(t *testing.T) {
	t.Parallel()
	spans, err := parseSpans(strings.NewReader(sample), spanFilter{})
	require.NoError(t, err)
	require.Len(t, spans, 5)
}

func TestParseSpansFiltersByScopeAndAge(t *testing.T) {
	t.Parallel()
	spans, err := parseSpans(strings.NewReader(sample), spanFilter{scope: "terminal"})
	require.NoError(t, err)
	require.Len(t, spans, 3)

	spans, err = parseSpans(strings.NewReader(sample), spanFilter{since: 1500 * 1e6})
	require.NoError(t, err)
	require.Len(t, spans, 2)
}

func TestSummarizeGroupsAndTakesPercentiles(t *testing.T) {
	t.Parallel()
	spans, err := parseSpans(strings.NewReader(sample), spanFilter{})
	require.NoError(t, err)
	var echo summary
	for _, s := range summarize(spans) {
		if s.Key == "terminal input:echo" {
			echo = s
		}
	}
	require.Equal(t, summary{Key: "terminal input:echo", N: 3, P50: 30, P95: 60, Max: 60, Long: 1}, echo)
}

func TestWriteReportComparesScenariosAgainstABaseline(t *testing.T) {
	t.Parallel()
	current, err := parseSpans(strings.NewReader(sample), spanFilter{})
	require.NoError(t, err)
	baseline, err := parseSpans(strings.NewReader(strings.ReplaceAll(sample, `"mbPerSec":40`, `"mbPerSec":20`)), spanFilter{})
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, writeReport(&out, current, baseline))
	require.Contains(t, out.String(), "bench scenario wall")
	require.Regexp(t, `mbPerSec\s+40\s+was 20\s+\+100%`, out.String())
}
