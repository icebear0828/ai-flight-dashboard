package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
)

func TestDiscoverGeminiHistoryFilesOnlyReturnsReplayableJSONL(t *testing.T) {
	root := t.TempDir()
	geminiDir := filepath.Join(root, ".gemini", "tmp", "wiki", "chats")
	claudeDir := filepath.Join(root, ".claude", "projects", "-Users-c-token")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	geminiA := filepath.Join(geminiDir, "a.jsonl")
	geminiB := filepath.Join(geminiDir, "b.jsonl")
	geminiUsage := `{"id":"abc","timestamp":"2026-05-01T02:44:45.432Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250},"model":"gemini-3.1-pro-preview"}` + "\n"
	for _, path := range []string{
		geminiA,
		geminiB,
	} {
		if err := os.WriteFile(path, []byte(geminiUsage), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(geminiDir, "ignored.txt"),
		filepath.Join(geminiDir, "unparseable.jsonl"),
		filepath.Join(claudeDir, "session.jsonl"),
	} {
		if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := discoverGeminiHistoryFiles([]string{root, filepath.Join(root, ".gemini", "tmp")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{geminiA, geminiB}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected Gemini history files:\nwant: %#v\n got: %#v", want, files)
	}
}

func TestLocalRepairDeviceIDsIncludesCurrentAndLegacyLocalOnce(t *testing.T) {
	if got, want := localRepairDeviceIDs("macbook"), []string{"macbook", "local", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected device IDs: want %#v got %#v", want, got)
	}
	if got, want := localRepairDeviceIDs("local"), []string{"local", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected legacy local device IDs: want %#v got %#v", want, got)
	}
}

func TestNewLANInstanceAdvertisesSyncPortWithoutTokenForZeroConfigLAN(t *testing.T) {
	enabled := true
	lanInst := newLANInstance(true, &enabled, "", "local-device", "19100")
	if lanInst == nil {
		t.Fatal("expected zero-config LAN instance without token")
	}
	if lanInst.DeviceID != "local-device" {
		t.Fatalf("unexpected device ID: %q", lanInst.DeviceID)
	}
	if lanInst.HTTPPort != 19100 {
		t.Fatalf("expected advertised zero-config sync port without token, got %d", lanInst.HTTPPort)
	}
}

func TestNewLANInstanceDefaultsMissingSettingToEnabled(t *testing.T) {
	lanInst := newLANInstance(true, nil, "", "local-device", "19100")
	if lanInst == nil {
		t.Fatal("expected LAN discovery to default on when setting is missing")
	}
	if lanInst.DeviceID != "local-device" {
		t.Fatalf("unexpected device ID: %q", lanInst.DeviceID)
	}
}

func TestNewLANInstanceAdvertisesSyncPortWithToken(t *testing.T) {
	enabled := true
	lanInst := newLANInstance(true, &enabled, "secret-token", "local-device", "19100")
	if lanInst == nil {
		t.Fatal("expected LAN instance with token")
	}
	if lanInst.HTTPPort != 19100 {
		t.Fatalf("expected advertised sync port, got %d", lanInst.HTTPPort)
	}
}

func TestQueueCodexTelemetryBackfillResetsOffsetOnce(t *testing.T) {
	database := testutil.NewTestDB(t)
	if err := database.SetOffset(codexusage.OffsetKey, 12345); err != nil {
		t.Fatal(err)
	}

	queueCodexTelemetryBackfill(database)

	offset, err := database.GetOffset(codexusage.OffsetKey)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatalf("expected Codex offset reset to 0, got %d", offset)
	}
	done, err := database.GetOffset(codexTelemetryBackfillMigrationKey)
	if err != nil {
		t.Fatal(err)
	}
	if done != 1 {
		t.Fatalf("expected Codex backfill migration marked done, got %d", done)
	}

	if err := database.SetOffset(codexusage.OffsetKey, 67890); err != nil {
		t.Fatal(err)
	}
	queueCodexTelemetryBackfill(database)
	offset, err = database.GetOffset(codexusage.OffsetKey)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 67890 {
		t.Fatalf("expected Codex offset unchanged after migration is done, got %d", offset)
	}
}

func TestNewLANInstanceDisabledBySettings(t *testing.T) {
	disabled := false
	if lanInst := newLANInstance(true, &disabled, "", "local-device", "19100"); lanInst != nil {
		t.Fatalf("expected LAN instance to be disabled, got %+v", lanInst)
	}
}

func TestStartLocalLANServicesStartsHTTPWithoutSyncToken(t *testing.T) {
	lanInst := lan.New("local-device", 19100)
	broadcastChan := make(chan model.TokenUsage)
	usageChan := make(chan model.TokenUsage)
	var capturedHandler http.Handler
	httpStarts := 0
	runtimeStarts := 0

	ok := startLocalLANServices(
		context.Background(),
		lanInst,
		nil,
		"",
		"19100",
		broadcastChan,
		usageChan,
		func(_ context.Context, port string, handler http.Handler) bool {
			httpStarts++
			if port != "19100" {
				t.Fatalf("expected LAN HTTP port 19100, got %q", port)
			}
			capturedHandler = handler
			return true
		},
		func(_ context.Context, gotLAN *lan.LAN, _ *db.DB, token string, gotBroadcast <-chan model.TokenUsage, gotUsage chan<- model.TokenUsage) {
			runtimeStarts++
			if gotLAN != lanInst {
				t.Fatalf("unexpected LAN instance: %+v", gotLAN)
			}
			if token != "" {
				t.Fatalf("expected empty sync token, got %q", token)
			}
			if gotBroadcast != broadcastChan {
				t.Fatal("unexpected broadcast channel")
			}
			if gotUsage != usageChan {
				t.Fatal("unexpected usage channel")
			}
		},
	)
	if !ok {
		t.Fatal("expected LAN services to start")
	}
	if httpStarts != 1 || runtimeStarts != 1 {
		t.Fatalf("expected one HTTP start and one runtime start, got http=%d runtime=%d", httpStarts, runtimeStarts)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/lan/self", nil)
	rec := httptest.NewRecorder()
	capturedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected LAN self endpoint to be available without token, got %d", rec.Code)
	}
	var self model.LANSelfResponse
	if err := json.NewDecoder(rec.Body).Decode(&self); err != nil {
		t.Fatal(err)
	}
	if self.DeviceID != "local-device" || self.HTTPPort != 19100 {
		t.Fatalf("unexpected LAN self response: %+v", self)
	}
}

func TestRuntimeLANControllerJoinLeaveRestartsLANRuntime(t *testing.T) {
	configDir := t.TempDir()
	config.SetDataDir(configDir)
	defer config.SetDataDir("")

	database := testutil.NewTestDB(t)
	defer database.Close()

	broadcastChan := make(chan model.TokenUsage)
	usageChan := make(chan model.TokenUsage)
	httpStarts := 0
	runtimeStarts := 0
	var runtimeContexts []context.Context

	controller := newRuntimeLANController(
		context.Background(),
		true,
		"local-device",
		"19100",
		"",
		database,
		broadcastChan,
		usageChan,
		func(ctx context.Context, port string, handler http.Handler) bool {
			httpStarts++
			runtimeContexts = append(runtimeContexts, ctx)
			if port != "19100" {
				t.Fatalf("expected port 19100, got %q", port)
			}
			if handler == nil {
				t.Fatal("expected LAN HTTP handler")
			}
			return true
		},
		func(ctx context.Context, gotLAN *lan.LAN, gotDB *db.DB, token string, gotBroadcast <-chan model.TokenUsage, gotUsage chan<- model.TokenUsage) {
			runtimeStarts++
			runtimeContexts = append(runtimeContexts, ctx)
			if gotLAN == nil || gotLAN.DeviceID != "local-device" {
				t.Fatalf("unexpected LAN instance: %+v", gotLAN)
			}
			if gotDB != database {
				t.Fatal("unexpected database")
			}
			if token != "" {
				t.Fatalf("expected empty token, got %q", token)
			}
			if gotBroadcast != broadcastChan {
				t.Fatal("unexpected broadcast channel")
			}
			if gotUsage != usageChan {
				t.Fatal("unexpected usage channel")
			}
		},
	)

	status, err := controller.Join()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || controller.CurrentLAN() == nil {
		t.Fatalf("expected LAN enabled after join, status=%+v current=%+v", status, controller.CurrentLAN())
	}
	if httpStarts != 1 || runtimeStarts != 1 {
		t.Fatalf("expected one HTTP and runtime start, got http=%d runtime=%d", httpStarts, runtimeStarts)
	}

	status, err = controller.Leave()
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || controller.CurrentLAN() != nil {
		t.Fatalf("expected LAN disabled after leave, status=%+v current=%+v", status, controller.CurrentLAN())
	}
	for _, ctx := range runtimeContexts {
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("expected runtime context to be canceled after leave")
		}
	}

	status, err = controller.Join()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || controller.CurrentLAN() == nil {
		t.Fatalf("expected LAN enabled after rejoin, status=%+v current=%+v", status, controller.CurrentLAN())
	}
	if httpStarts != 2 || runtimeStarts != 2 {
		t.Fatalf("expected LAN runtime to restart, got http=%d runtime=%d", httpStarts, runtimeStarts)
	}
}

func TestRunRepairHistorySupersedesLocalGeminiLegacyRows(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	root := t.TempDir()
	geminiDir := filepath.Join(root, ".gemini", "tmp", "wiki", "chats")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}
	geminiFile := filepath.Join(geminiDir, "session.jsonl")
	geminiUsage := `{"timestamp":"2026-05-07T11:13:03.316Z","type":"gemini","tokens":{"input":1000,"output":50,"cached":250},"model":"gemini-2.5-pro"}` + "\n"
	if err := os.WriteFile(geminiFile, []byte(geminiUsage), 0644); err != nil {
		t.Fatal(err)
	}

	oldTS := time.Date(2026, 5, 7, 11, 13, 3, 0, time.UTC)
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 800, OutputTokens: 500},
		10.00, oldTS, geminiFile, "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 700, OutputTokens: 400},
		20.00, oldTS, geminiFile, "remote-mac",
	); err != nil {
		t.Fatal(err)
	}

	runRepairHistory(database, calc, "local", []string{root})

	localCost, localIn, localCached, _, localOut, err := database.QueryPeriodStatsAll("local", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if localCost <= 0 || localIn != 1000 || localCached != 250 || localOut != 50 {
		t.Fatalf("expected local stats from repaired active row only, cost=%f input=%d cached=%d output=%d", localCost, localIn, localCached, localOut)
	}

	remoteCost, remoteIn, _, _, remoteOut, err := database.QueryPeriodStatsAll("remote-mac", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if remoteCost != 20.00 || remoteIn != 700 || remoteOut != 400 {
		t.Fatalf("expected remote legacy row to remain active, cost=%f input=%d output=%d", remoteCost, remoteIn, remoteOut)
	}
}

func TestRunRepairHistoryWithNoOptionalSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	runRepairHistory(database, calc, "local", nil)

	for _, source := range []string{"Claude Code", "Gemini CLI", "Codex"} {
		cost, input, cached, cacheCreation, output, err := database.QueryPeriodStatsAll("", source)
		if err != nil {
			t.Fatal(err)
		}
		if cost != 0 || input != 0 || cached != 0 || cacheCreation != 0 || output != 0 {
			t.Fatalf("expected no %s rows, cost=%f input=%d cached=%d cacheCreation=%d output=%d", source, cost, input, cached, cacheCreation, output)
		}
	}
}
