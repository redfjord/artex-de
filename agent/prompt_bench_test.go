package agent

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Autumn-27/artex/db"
)

var benchmarkSystem []string
var benchmarkBoundary int

func BenchmarkWorkerPromptAssembly(b *testing.B) {
	for _, count := range []int{0, 64} {
		b.Run(fmt.Sprintf("deferred_%d", count), func(b *testing.B) {
			def := DeferredInfo{FindingGuidance: "\nEvidence must come from the current intent."}
			for i := 0; i < count; i++ {
				def.GlobalNames = append(def.GlobalNames, fmt.Sprintf("mcp__fixture__tool_%03d", i))
			}
			intent := &db.Node{ID: 11, Payload: json.RawMessage(`{"summary":"probe local fixture"}`)}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				base := workerSystem("127.0.0.1:8788", "/fixture/ca.pem", "/data")
				tail := workerArtifactSpec("/data/tasks/1/i11") + renderIntentTask(intent) + "\nOnly execute this intent."
				benchmarkSystem, benchmarkBoundary = deferredSystemWithTail(base, tail, def)
			}
		})
	}
}
