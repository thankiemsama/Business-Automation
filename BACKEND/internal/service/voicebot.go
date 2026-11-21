// Package service – P4 Voicebot: inbound/outbound AI voice calls via Twilio.
//
// The voicebot reuses Nexa's LLM knowledge provider to answer each turn, Twilio
// server-side STT (<Record transcribe>) to capture customer speech, and TwiML
// <Say> for TTS responses. Inbound calls POST to the tenant's webhook handler;
// outbound calls are initiated against the Twilio REST API with a TwiML URL
// that Twilio fetches.
//
// Governance: every spoken turn checks GovernanceAllows + AgentCanAutoAct for
// the "voicebot" agent before generating an answer; kill-switched or
// approval-gated tenants get a polite hang-up instead.
package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

// voicebotCreds holds the per-tenant Twilio Voice credentials.
type voicebotCreds struct {
	AccountSID  string `json:"account_sid"`
	AuthToken   string `json:"auth_token"`
	FromNumber  string `json:"from_number"`
	MaxDuration int    `json:"max_duration"` // seconds; 0 = default 600
}

// VoicebotCredStore decodes and validates voicebot credentials from environment.
type VoicebotCredStore struct {
	creds  map[string]voicebotCreds
	twilio *http.Client
	origin string
}

// NewVoicebotCredStore reads NEXA_VOICEBOT_CREDENTIALS (JSON) and the app origin.
func NewVoicebotCredStore(origin string) *VoicebotCredStore {
	s := &VoicebotCredStore{
		creds: map[string]voicebotCreds{},
		twilio: &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		origin: origin,
	}
	var raw map[string]voicebotCreds
	if err := json.Unmarshal([]byte(os.Getenv("NEXA_VOICEBOT_CREDENTIALS")), &raw); err == nil {
		for tenant, c := range raw {
			if c.AccountSID == "" || c.AuthToken == "" {
				continue
			}
			if c.MaxDuration <= 0 {
				c.MaxDuration = 600
			}
			s.creds[tenant] = c
		}
	}
	return s
}

// Creds returns the credentials for a tenant.
func (s *VoicebotCredStore) Creds(tenant string) (voicebotCreds, bool) {
	c, ok := s.creds[tenant]
	return c, ok
}

// WebhookURL is the callback URL Twilio POSTs to for this tenant's calls.
func (s *VoicebotCredStore) WebhookURL(tenant string) string {
	return s.origin + "/api/v1/voicebot/calls/" + url.PathEscape(tenant)
}

// TwimlURL is the GET endpoint Twilio fetches for outbound-call TwiML.
func (s *VoicebotCredStore) TwimlURL(tenant string, ref string) string {
	return s.origin + "/api/v1/voicebot/twiml/" + url.PathEscape(tenant) + "/" + url.PathEscape(ref)
}

// ValidateWebhookSignature verifies a Twilio X-Twilio-Signature header.
// The HMAC-SHA1 key is the auth token; the payload is the callback URL
// followed by each sorted key=value form pair concatenated.
func (s *VoicebotCredStore) ValidateWebhookSignature(c voicebotCreds, callbackURL string, headers http.Header, raw []byte) bool {
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return false
	}
	keys := make([]string, 0, len(values))
	for k, v := range values {
		if len(v) != 1 {
			return false
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	mac := hmac.New(sha1.New, []byte(c.AuthToken))
	_, _ = io.WriteString(mac, callbackURL)
	for _, k := range keys {
		_, _ = io.WriteString(mac, k)
		_, _ = io.WriteString(mac, values.Get(k))
	}
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	supplied := headers.Get("X-Twilio-Signature")
	return supplied != "" && hmac.Equal([]byte(expected), []byte(supplied))
}

// ---- Call state -----------------------------------------------------------

// CallState is one voicebot call, keyed deterministically by Twilio CallSid.
type CallState struct {
	ID        string    `json:"id"`
	CallSID   string    `json:"call_sid"`
	TenantID  string    `json:"tenant_id"`
	Caller    string    `json:"caller"`
	Callee    string    `json:"callee"`
	Direction string    `json:"direction"` // "inbound" | "outbound"
	Status    string    `json:"status"`    // "active" | "ended"
	Turn      uint64    `json:"turn"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const voicebotCallKind = "voicebot_calls"

var e164Strict = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

func voicebotCallID(callSID string) string { return tokenHash("voicebot-call:" + callSID) }

// upsertCallState persists a CallState as a core.Record (idempotent on CallSID).
func upsertCallState(store core.Writer, state *CallState) {
	store.PutRecord(core.Record{
		ID:             state.ID,
		TenantID:       state.TenantID,
		Kind:           voicebotCallKind,
		Name:           state.Caller,
		Content:        state.Callee,
		Channel:        core.ChannelPhone,
		AutomationID:   state.Direction + ":" + state.Status,
		ConversationID: state.CallSID,
		Sequence:       state.Turn,
		CreatedAt:      state.CreatedAt,
		UpdatedAt:      state.UpdatedAt,
	})
}

// ---- Voicebot service ------------------------------------------------------

// Voicebot handles inbound Twilio Voice webhooks and outbound AI calls.
type Voicebot struct {
	credStore *VoicebotCredStore
	repo      *Workspace
	clock     Clock
	model     *OpenAIKnowledge
}

// NewVoicebot builds the voicebot service.
func NewVoicebot(repo *Workspace, clock Clock, model *OpenAIKnowledge, credStore *VoicebotCredStore) *Voicebot {
	return &Voicebot{credStore: credStore, repo: repo, clock: clock, model: model}
}

// Enabled checks whether the voicebot is available for a tenant: the AI model is
// configured, the ai_voicebot flag is opted in, and governance has not killed it.
func (v *Voicebot) Enabled(user core.User) bool {
	if v.model == nil {
		return false
	}
	enabled := false
	_ = v.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		enabled = aiOptIn(store, user.TenantID, "ai_voicebot") &&
			GovernanceAllows(store, user.TenantID, "voicebot")
		return nil
	})
	return enabled
}

// InboundCall handles a Twilio Voice webhook POST and returns a TwiML document
// for Twilio to execute. Twilio sends form-encoded data (CallSid, From, To,
// Transcription, ...); after <Record transcribe> the next POST carries the
// transcription of the caller's utterance.
func (v *Voicebot) InboundCall(tenant string, header http.Header, raw []byte) (string, error) {
	creds, ok := v.credStore.Creds(tenant)
	if !ok {
		return "", core.Fail(503, "Voicebot chưa cấu hình cho tenant")
	}
	callbackURL := v.credStore.WebhookURL(tenant)
	if !v.credStore.ValidateWebhookSignature(creds, callbackURL, header, raw) {
		return "", core.Fail(401, "Ký hiệu Twilio không hợp lệ")
	}

	values, _ := url.ParseQuery(string(raw))
	callSID := values.Get("CallSid")
	from := values.Get("From")
	to := values.Get("To")
	transcription := strings.TrimSpace(values.Get("Transcription"))
	if callSID == "" || !e164Strict.MatchString(from) {
		return "", core.Fail(400, "Dữ liệu cuộc gọi Twilio không hợp lệ")
	}

	// Load or create the call state, increment the turn counter.
	var state CallState
	err := updateTenant(v.repo, tenant, func(store core.Writer) error {
		id := voicebotCallID(callSID)
		now := v.clock.Now().UTC()
		previous, found := store.Record(tenant, id, voicebotCallKind)
		if !found {
			state = CallState{
				ID: id, CallSID: callSID, TenantID: tenant,
				Caller: from, Callee: to, Direction: "inbound",
				Status: "active", Turn: 1, CreatedAt: now, UpdatedAt: now,
			}
		} else {
			direction := "inbound"
			if strings.HasPrefix(previous.AutomationID, "outbound:") {
				direction = "outbound"
			}
			state = CallState{
				ID: id, CallSID: callSID, TenantID: tenant,
				Caller: previous.Name, Callee: previous.Content,
				Direction: direction, Status: "active",
				Turn:      previous.Sequence + 1,
				CreatedAt: previous.CreatedAt, UpdatedAt: now,
			}
		}
		upsertCallState(store, &state)
		return nil
	})
	if err != nil {
		return twiMLHangup("Lỗi nội bộ, xin vui lòng gọi lại sau."), nil
	}

	// Governance gate: a killed or approval-gated voicebot must not speak.
	gated := false
	_ = v.repo.ViewTenant(tenant, func(store core.Reader) error {
		if !AgentCanAutoAct(store, tenant, "voicebot") {
			gated = true
		}
		return nil
	})
	if gated {
		return twiMLHangup("Hơi tiếc là hệ thống đang tạm ngừng phục vụ. Cảm ơn quý khách đã gọi, xin vui lòng thử lại sau."), nil
	}

	// First turn (no transcription yet): play the greeting and start recording.
	if transcription == "" {
		return v.greetingTwiML(callbackURL, state, tenant), nil
	}

	// Subsequent turns: answer the transcription with the AI knowledge provider.
	answer, handoff, aiErr := v.aiAnswer(tenant, transcription)
	if aiErr != nil || handoff || answer == "" {
		msg := answer
		if msg == "" {
			msg = "Xin chuyển quý khách đến nhân viên hỗ trợ. Cảm ơn quý khách đã gọi."
		}
		return twiMLSayAndRecord(msg, callbackURL, state, tenant, 0, false), nil
	}

	// Persist the turn into the call's phone conversation for observability.
	_ = v.repo.UpdateTenant(tenant, func(store core.Writer) error {
		v.persistTurn(store, tenant, state, from, transcription, answer)
		return nil
	})

	return twiMLSayAndRecord(answer, callbackURL, state, tenant, 30, true), nil
}

// persistTurn stores the customer transcription and the AI reply as messages in
// the call's phone conversation (creating the customer + conversation if new).
func (v *Voicebot) persistTurn(store core.Writer, tenant string, state CallState, caller, transcription, answer string) {
	convID := tokenHash("voicebot-conversation:" + state.CallSID)
	conversation, ok := store.Record(tenant, convID, core.KindConversations)
	now := v.clock.Now().UTC()
	if !ok {
		customerID := tokenHash("voicebot-customer:" + tenant + ":" + caller)
		customer, found := store.Record(tenant, customerID, core.KindCustomers)
		if !found {
			customer = core.Record{ID: v.repo.IDs.NewID(), TenantID: tenant, Kind: core.KindCustomers, Name: caller, CreatedAt: now, UpdatedAt: now}
			store.PutRecord(customer)
		}
		conversation = core.Record{
			ID: convID, TenantID: tenant, Kind: core.KindConversations,
			CustomerID: customer.ID, Channel: core.ChannelPhone,
			Subject: "Gọi · " + caller, Status: core.StatusOpen,
			CreatedAt: now, UpdatedAt: now,
		}
		store.PutRecord(conversation)
	}

	// Customer's spoken words.
	base := nextSequence(store, core.User{TenantID: tenant}, conversation.ID)
	store.PutRecord(core.Record{
		ID: v.repo.IDs.NewID(), TenantID: tenant, Kind: core.KindMessages,
		ConversationID: conversation.ID, Sender: core.SenderCustomer,
		Content: transcription, Sequence: base,
		CreatedAt: now, UpdatedAt: now,
	})
	// AI's spoken answer.
	store.PutRecord(core.Record{
		ID: v.repo.IDs.NewID(), TenantID: tenant, Kind: core.KindMessages,
		ConversationID: conversation.ID, Sender: core.SenderAssistant,
		Content: answer, Sequence: base + 1,
		CreatedAt: now, UpdatedAt: now,
	})
	conversation.UpdatedAt = now
	store.PutRecord(conversation)
}

// OutboundCall initiates an outbound AI voice call to a customer number.
// Twilio fetches the tenant's TwiML endpoint, plays the greeting, records the
// response, and POSTs back to the inbound handler for the conversation loop.
func (v *Voicebot) OutboundCall(user core.User, to, greeting string) (string, error) {
	if user.Role != core.RoleOwner {
		return "", core.Fail(403, "Chỉ owner phát lệnh gọi ra")
	}
	if !e164Strict.MatchString(to) {
		return "", core.Fail(400, "Số điện thoại phải ở dạng E.164 (+84...)")
	}
	creds, ok := v.credStore.Creds(user.TenantID)
	if !ok {
		return "", core.Fail(503, "Voicebot chưa cấu hình Twilio cho tenant")
	}
	if !v.Enabled(user) {
		return "", core.Fail(503, "Voicebot chưa được bật")
	}

	ref := v.repo.IDs.NewID()
	twiMLURL := v.credStore.TwimlURL(user.TenantID, ref)
	// Attach the greeting so Twilio's first-turn fetch renders the correct <Say>.
	if greeting != "" {
		parsed, err := url.Parse(twiMLURL)
		if err == nil {
			q := parsed.Query()
			q.Set("greeting", greeting)
			parsed.RawQuery = q.Encode()
			twiMLURL = parsed.String()
		}
	}
	sid, err := v.initiateCall(creds, to, twiMLURL)
	if err != nil {
		return "", err
	}

	now := v.clock.Now().UTC()
	_ = v.repo.UpdateTenant(user.TenantID, func(store core.Writer) error {
		id := voicebotCallID(sid)
		state := CallState{
			ID: id, CallSID: sid, TenantID: user.TenantID,
			Caller: creds.FromNumber, Callee: to,
			Direction: "outbound", Status: "active",
			Turn: 0, CreatedAt: now, UpdatedAt: now,
		}
		upsertCallState(store, &state)
		return nil
	})
	return sid, nil
}

// CallHistory lists the most recent voicebot calls for the tenant.
func (v *Voicebot) CallHistory(user core.User) ([]CallState, error) {
	if user.Role != core.RoleOwner && user.Role != core.RoleAgent {
		return nil, core.Fail(403, "Không có quyền xem lịch sử cuộc gọi")
	}
	result := []CallState{}
	err := v.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, voicebotCallKind) {
			direction, status := "inbound", "active"
			if i := strings.IndexByte(record.AutomationID, ':'); i >= 0 {
				direction, status = record.AutomationID[:i], record.AutomationID[i+1:]
			}
			result = append(result, CallState{
				ID: record.ID, CallSID: record.ConversationID, TenantID: record.TenantID,
				Caller: record.Name, Callee: record.Content,
				Direction: direction, Status: status,
				Turn: record.Sequence, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
			})
		}
		return nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	if len(result) > 100 {
		result = result[:100]
	}
	return result, err
}

// initiateCall POSTs to Twilio's Create Call REST endpoint.
func (v *Voicebot) initiateCall(creds voicebotCreds, to, twiMLURL string) (string, error) {
	form := url.Values{
		"To":                  {to},
		"From":                {creds.FromNumber},
		"Url":                 {twiMLURL},
		"Method":              {"GET"},
		"MaxDuration":         {fmt.Sprint(creds.MaxDuration)},
		"StatusCallbackEvent": {"completed"},
	}
	endpoint := "https://api.twilio.com/2010-04-01/Accounts/" + creds.AccountSID + "/Calls.json"
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", core.Fail(502, "Không thể tạo yêu cầu Twilio")
	}
	req.SetBasicAuth(creds.AccountSID, creds.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := v.credStore.twilio.Do(req)
	if err != nil {
		return "", core.Fail(502, "Không kết nối được Twilio")
	}
	defer response.Body.Close()

	var res struct {
		SID  string `json:"sid"`
		Errs []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&res) != nil {
		return "", core.Fail(502, "Phản hồi Twilio không hợp lệ")
	}
	if response.StatusCode >= 400 || res.SID == "" {
		msg := "Lỗi Twilio"
		if len(res.Errs) > 0 {
			msg = res.Errs[0].Message
		}
		return "", core.Fail(502, msg)
	}
	return res.SID, nil
}

// aiAnswer generates a spoken response for a transcription using the knowledge
// base and the CS persona SOP. Returns (answer, needsHandoff, error).
func (v *Voicebot) aiAnswer(tenant, transcription string) (string, bool, error) {
	if v.model == nil {
		return "", true, core.Fail(503, "Nhà cung cấp AI chưa cấu hình")
	}
	var articles []core.Record
	var csSOP string
	_ = v.repo.ViewTenant(tenant, func(store core.Reader) error {
		for _, article := range store.Records(tenant, core.KindKnowledge) {
			if article.Public {
				articles = append(articles, article)
			}
		}
		settings, _ := tenantSettings(store, tenant)
		if aiOptIn(store, tenant, "ai_cs_persona") {
			csSOP = settings.PersonaSOP["cs"]
		}
		if skill, ok := MatchedSkill(store, tenant, core.ChannelPhone, transcription); ok {
			if csSOP != "" {
				csSOP += "\n"
			}
			csSOP += skill.SOP()
		}
		return nil
	})

	answer, err := v.model.answerWithSOP(tenant, transcription, articles, csSOP)
	if err != nil {
		return "", true, err
	}
	if answer.NeedsHuman {
		return answer.Content, true, nil
	}
	// For voice, keep answers concise since TTS reads them aloud.
	answer.Content = clipped(answer.Content, 600)
	if answer.Content == "" {
		return humanHandoffMessage, true, nil
	}
	return answer.Content, false, nil
}

// TwimlForOutbound renders the TwiML document for an outbound call's first turn.
func (v *Voicebot) TwimlForOutbound(tenant, ref, to, greeting string) string {
	creds, _ := v.credStore.Creds(tenant)
	_ = ref
	return twiMLOutbound(to, greeting, v.credStore.WebhookURL(tenant), creds, tenant)
}

// ---- TwiML generators ------------------------------------------------------

// greetingTwiML returns a <Response> that plays the welcome and starts recording.
func (v *Voicebot) greetingTwiML(callbackURL string, state CallState, tenant string) string {
	greeting := "Xin chào, cảm ơn quý khách đã gọi. Hệ thống đang thu nghe câu hỏi của quý khách, xin vui lòng nói sau khi nghe tiếng bíp."
	return twiMLSayAndRecord(greeting, callbackURL, state, tenant, 20, true)
}

// twiMLSayAndRecord builds a <Response> with <Say> and an optional <Record>.
// When record is true, <Record transcribe="true"> captures and transcribes the
// caller's next utterance and POSTs back to callbackURL.
func twiMLSayAndRecord(say, callbackURL string, state CallState, tenant string, timeout int, record bool) string {
	const lang = "vi-VN"
	const voice = "alice"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Response>`)
	if say != "" {
		b.WriteString(`<Say voice="` + voice + `" language="` + lang + `">`)
		b.WriteString(escapeXML(say))
		b.WriteString(`</Say>`)
	}
	if record {
		b.WriteString(`<Record action="` + escapeXML(callbackURL) + `" timeout="` + fmt.Sprint(timeout) +
			`" transcribe="true" language="` + lang + `" digits="` + fmt.Sprint(state.Turn) + `"/>`)
	}
	b.WriteString(`</Response>`)
	return b.String()
}

// twiMLOutbound builds the first-turn TwiML for an outbound call: play the
// greeting, then record the customer's response and POST back to callbackURL.
func twiMLOutbound(to, greeting, callbackURL string, creds voicebotCreds, tenant string) string {
	const lang = "vi-VN"
	const voice = "alice"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Response>`)
	if greeting != "" {
		b.WriteString(`<Say voice="` + voice + `" language="` + lang + `">`)
		b.WriteString(escapeXML(greeting))
		b.WriteString(`</Say>`)
	}
	b.WriteString(`<Record action="` + escapeXML(callbackURL) + `" timeout="20" transcribe="true" language="` + lang + `" digits="0"/>`)
	b.WriteString(`</Response>`)
	return b.String()
}

// twiMLHangup returns a <Response> that plays a message and ends the call.
func twiMLHangup(message string) string {
	const lang = "vi-VN"
	const voice = "alice"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Response>`)
	b.WriteString(`<Say voice="` + voice + `" language="` + lang + `">`)
	b.WriteString(escapeXML(message))
	b.WriteString(`</Say><Hangup/></Response>`)
	return b.String()
}

// escapeXML escapes a string for use inside an XML text node.
func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}
