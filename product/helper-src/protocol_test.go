package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestCanonicalContractSchema(t *testing.T) {
	var spec struct {
		ProtocolVersion    int      `json:"protocolVersion"`
		JobStates          []string `json:"jobStates"`
		DownloadStrategies []string `json:"downloadStrategies"`
		Commands           []string `json:"commands"`
		Events             []string `json:"events"`
		ErrorCodes         []string `json:"errorCodes"`
		Capabilities       []string `json:"capabilities"`
	}
	b, err := os.ReadFile("../contracts/contracts-0.5.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocol version %d", spec.ProtocolVersion)
	}
	for _, state := range spec.JobStates {
		if stateForEvent(map[string]string{"queued": "queued", "analyzing": "job_started", "downloading": "progress", "processing": "progress", "completed": "complete", "failed": "error", "cancelled": "cancelled"}[state], map[string]string{"downloading": "تنزيل مباشر", "processing": "دمج الفيديو والصوت"}[state]) == "" && state != "interrupted" {
			t.Fatalf("unmapped state %q", state)
		}
	}
	for _, command := range spec.Commands {
		if !validCommand(command) {
			t.Fatalf("invalid command %q", command)
		}
	}
	for _, event := range spec.Events {
		if !validEvent(event) {
			t.Fatalf("invalid event %q", event)
		}
	}
	wantErrors := map[string]bool{}
	for _, v := range []ErrorCode{ErrorInvalidRequest, ErrorUnsupported, ErrorAuthenticationRequired, ErrorUnavailable, ErrorDRMProtected, ErrorExpiredURL, ErrorHTTP403, ErrorExtractionFailed, ErrorCancelled, ErrorToolMissing, ErrorIO, ErrorUpdateFailed, ErrorInternal} {
		wantErrors[string(v)] = true
	}
	for _, v := range spec.ErrorCodes {
		if !wantErrors[v] {
			t.Fatalf("unknown error code %q", v)
		}
	}
	if len(spec.Capabilities) != 12 {
		t.Fatalf("capability schema mismatch: %v", spec.Capabilities)
	}
	if len(spec.DownloadStrategies) != 3 || strategyForKind("download_detected") != StrategyDirectHTTP || strategyForKind("download_video") != StrategyYTDLP || strategyForKind("convert_mp3") != StrategyFFmpeg {
		t.Fatal("strategy contract mismatch")
	}
}

func TestNativeWriterEnforcesProtocolAndProgress(t *testing.T) {
	var wire bytes.Buffer
	nw := &nativeWriter{w: &wire}
	if err := nw.send(Response{Event: "progress", Kind: "download_detected", Progress: 100}); err != nil {
		t.Fatal(err)
	}
	r := responses(t, wire.Bytes())[0]
	if r.ProtocolVersion != ProtocolVersion || r.Progress != 99.5 || r.State != JobDownloading || r.Strategy != StrategyDirectHTTP {
		t.Fatalf("unexpected response %+v", r)
	}
}
