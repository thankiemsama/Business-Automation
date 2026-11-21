package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

// twilioSignature signs a Twilio webhook form body the way Twilio does:
// HMAC-SHA1 over (callbackURL + each sorted key + its value), base64 encoded.
func twilioSignature(creds voicebotCreds, callbackURL string, form url.Values) string {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	mac := hmac.New(sha1.New, []byte(creds.AuthToken))
	_, _ = io.WriteString(mac, callbackURL)
	for _, k := range keys {
		_, _ = io.WriteString(mac, k)
		_, _ = io.WriteString(mac, form.Get(k))
	}
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func newVoicebotTest(t *testing.T, origin string, creds map[string]voicebotCreds) (*VoicebotCredStore, *Voicebot, *Workspace) {
	t.Helper()
	raw, _ := json.Marshal(creds)
	t.Setenv("NEXA_VOICEBOT_CREDENTIALS", string(raw))
	credStore := NewVoicebotCredStore(origin)
	base, err := repository.Open(filepath.Join(t.TempDir(), "voicebot.json"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace(base, &workspaceClock{now: time.Now().UTC()}, RandomIDs{})
	voicebot := NewVoicebot(workspace, workspace.Clock, nil, credStore)
	return credStore, voicebot, workspace
}

// TestVoicebotCredentialStore verifies NEXA_VOICEBOT_CREDENTIALS is parsed and
// that tenants missing credentials are rejected.
func TestVoicebotCredentialStore(t *testing.T) {
	store, _, _ := newVoicebotTest(t, "https://nexa.test", map[string]voicebotCreds{
		"tenant": {AccountSID: "AC0000000000000000000000000000", AuthToken: "secret", FromNumber: "+15550000000", MaxDuration: 300},
	})
	if _, ok := store.Creds("tenant"); !ok {
		t.Fatal("expected tenant credentials")
	}
	if _, ok := store.Creds("missing"); ok {
		t.Fatal("unexpected credentials for missing tenant")
	}
}

// TestVoicebotWebhookSignature verifies HMAC-SHA1 webhook signature checks.
func TestVoicebotWebhookSignature(t *testing.T) {
	creds := voicebotCreds{AccountSID: "AC1", AuthToken: "secret", FromNumber: "+15550000000"}
	callbackURL := "https://nexa.test/api/v1/voicebot/calls/tenant"
	store := NewVoicebotCredStore("https://nexa.test")
	store.creds["tenant"] = creds

	form := url.Values{
		"CallSid":       {"CA123"},
		"From":          {"+15550001111"},
		"To":            {"+15550000000"},
		"Transcription": {"xin chào giá điện"},
	}
	raw := form.Encode()
	headers := http.Header{}
	if store.ValidateWebhookSignature(creds, callbackURL, headers, []byte(raw)) {
		t.Fatal("unsigned webhook accepted")
	}
	sig := twilioSignature(creds, callbackURL, form)
	headers.Set("X-Twilio-Signature", sig)
	if !store.ValidateWebhookSignature(creds, callbackURL, headers, []byte(raw)) {
		t.Fatal("valid signature rejected")
	}
	// Tampered body must fail.
	if store.ValidateWebhookSignature(creds, callbackURL, headers, []byte(raw+"&Transcription=tampered")) {
		t.Fatal("tampered body accepted")
	}
}

// TestVoicebotTwiMLGeneration checks the TwiML documents are well-formed and
// carry the expected <Say>/<Record>/<Hangup> elements.
func TestVoicebotTwiMLGeneration(t *testing.T) {
	state := CallState{ID: "id", CallSID: "CA1", TenantID: "tenant", Caller: "+15550001111", Callee: "+15550000000", Direction: "inbound", Status: "active", Turn: 1}

	record := twiMLSayAndRecord("Cảm ơn quý khách", "https://nexa.test/cb", state, "tenant", 20, true)
	if !strings.Contains(record, `<Say voice="alice" language="vi-VN">`) || !strings.Contains(record, `transcribe="true"`) || !strings.Contains(record, `digits="1"`) {
		t.Fatalf("unexpected record TwiML: %s", record)
	}

	hangup := twiMLHangup("Hẹn gặp lại")
	if !strings.Contains(hangup, `<Hangup/>`) || !strings.Contains(hangup, "Hẹn gặp lại") {
		t.Fatalf("unexpected hangup TwiML: %s", hangup)
	}

	outbound := twiMLOutbound("+15550001111", "Xin chào, tôi gọi từ Nexa", "https://nexa.test/cb", voicebotCreds{}, "tenant")
	if !strings.Contains(outbound, `<Say`) || !strings.Contains(outbound, `<Record`) || !strings.Contains(outbound, "Nexa") {
		t.Fatalf("unexpected outbound TwiML: %s", outbound)
	}

	if got := escapeXML(`<A&"B">`); got != "&lt;A&amp;&quot;B&quot;&gt;" {
		t.Fatalf("escapeXML broken: %s", got)
	}
}

// TestVoicebotInboundGreetingAndKillSwitch verifies the first turn returns a
// greeting TwiML, and that a killed voicebot returns a polite hang-up.
func TestVoicebotInboundGreetingAndKillSwitch(t *testing.T) {
	creds := voicebotCreds{AccountSID: "AC1", AuthToken: "secret", FromNumber: "+15550000000", MaxDuration: 300}
	origin := "https://nexa.test"
	callbackURL := origin + "/api/v1/voicebot/calls/tenant"
	_, voicebot, workspace := newVoicebotTest(t, origin, map[string]voicebotCreds{"tenant": creds})

	owner := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error { store.PutUser(owner); return nil }); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"CallSid": {"CA999"}, "From": {"+15550001111"}, "To": {"+15550000000"}, "Transcription": {""}}
	raw := form.Encode()
	headers := http.Header{}
	headers.Set("X-Twilio-Signature", twilioSignature(creds, callbackURL, form))

	// First turn (empty transcription): expect a greeting with <Record>.
	twiML, err := voicebot.InboundCall("tenant", headers, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(twiML, `<Say`) || !strings.Contains(twiML, `<Record`) {
		t.Fatalf("expected greeting TwiML with record, got: %s", twiML)
	}

	// Kill the voicebot via governance; the next call must hang up politely.
	governance := NewGovernance(workspace, workspace.Clock)
	_, err = governance.Save(owner, AgentGovernance{
		PlatformKill: false,
		Disabled:     map[string]bool{"voicebot": true},
		RiskTiers:    map[string]string{},
		Approval:     map[string]bool{},
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	form2 := url.Values{"CallSid": {"CA999"}, "From": {"+15550001111"}, "To": {"+15550000000"}, "Transcription": {"xin chào"}}
	raw2 := form2.Encode()
	headers2 := http.Header{}
	headers2.Set("X-Twilio-Signature", twilioSignature(creds, callbackURL, form2))
	killed, err := voicebot.InboundCall("tenant", headers2, []byte(raw2))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(killed, `<Hangup/>`) || strings.Contains(killed, `<Record`) {
		t.Fatalf("killed voicebot should hang up, got: %s", killed)
	}
}

// TestVoicebotCallHistory verifies recorded calls are listed newest-first.
func TestVoicebotCallHistory(t *testing.T) {
	_, voicebot, workspace := newVoicebotTest(t, "https://nexa.test", map[string]voicebotCreds{})

	owner := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(owner)
		now := time.Now().UTC()
		upsertCallState(store, &CallState{ID: "c1", CallSID: "CA1", TenantID: "tenant", Caller: "+15550001111", Callee: "+15550000000", Direction: "inbound", Status: "active", Turn: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)})
		upsertCallState(store, &CallState{ID: "c2", CallSID: "CA2", TenantID: "tenant", Caller: "+15550002222", Callee: "+15550000000", Direction: "outbound", Status: "active", Turn: 0, CreatedAt: now, UpdatedAt: now})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	history, err := voicebot.CallHistory(owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 calls, got %d: %+v", len(history), history)
	}
	if history[0].CallSID != "CA2" || history[1].CallSID != "CA1" {
		t.Fatalf("calls not newest-first: %+v", history)
	}
}
