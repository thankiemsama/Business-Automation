package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"businessautomation/internal/core"
)

var e164Number = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
var twilioAccountSID = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
var twilioMessageSID = regexp.MustCompile(`^SM[0-9a-fA-F]{32}$`)

func validChannelAddress(channel, value string) bool {
	if channel == core.ChannelSMS {
		return e164Number.MatchString(value)
	}
	return channelID.MatchString(value)
}

func validSMSCredentials(tenant string, creds channelCredentials) bool {
	callback, err := url.Parse(creds.CallbackURL)
	return twilioAccountSID.MatchString(creds.TwilioSID) && err == nil &&
		callback.Scheme == "https" && callback.Hostname() != "" && callback.User == nil &&
		callback.RawQuery == "" && !callback.ForceQuery && callback.Fragment == "" &&
		callback.EscapedPath() == "/api/v1/channel-webhooks/"+url.PathEscape(tenant)+"/sms"
}

func parseSMS(account channelAccount, creds channelCredentials, headers http.Header, raw []byte) ([]inboundChannelMessage, error) {
	values, err := url.ParseQuery(string(raw))
	contentType, _, typeErr := mime.ParseMediaType(headers.Get("Content-Type"))
	invalid := core.Fail(401, "Webhook SMS không hợp lệ")
	if err != nil || typeErr != nil || contentType != "application/x-www-form-urlencoded" {
		return nil, invalid
	}
	keys := make([]string, 0, len(values))
	for key, entries := range values {
		if len(entries) != 1 {
			return nil, invalid
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	mac := hmac.New(sha1.New, []byte(creds.WebhookSecret))
	_, _ = io.WriteString(mac, creds.CallbackURL)
	for _, key := range keys {
		_, _ = io.WriteString(mac, key)
		_, _ = io.WriteString(mac, values.Get(key))
	}
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(headers.Get("X-Twilio-Signature"))) ||
		values.Get("To") != account.AccountID || values.Get("AccountSid") != creds.TwilioSID {
		return nil, invalid
	}
	id, from, body := values.Get("MessageSid"), values.Get("From"), strings.TrimSpace(values.Get("Body"))
	if !twilioMessageSID.MatchString(id) || !e164Number.MatchString(from) {
		return nil, core.Fail(400, "Dữ liệu SMS không hợp lệ")
	}
	if body == "" || values.Get("OptOutType") != "" {
		return nil, nil
	}
	switch strings.ToUpper(body) {
	case "STOP", "STOPALL", "UNSUBSCRIBE", "CANCEL", "END", "QUIT", "START", "YES", "UNSTOP", "HELP", "INFO":
		return nil, nil
	}
	return []inboundChannelMessage{{id: id, sender: from, text: body}}, nil
}

func sendSMS(ctx context.Context, client *http.Client, account channelAccount, creds channelCredentials, to, content string) channelSendResult {
	form := url.Values{"To": {to}, "From": {account.AccountID}, "Body": {content}}
	endpoint := "https://api.twilio.com/2010-04-01/Accounts/" + creds.TwilioSID + "/Messages.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return channelSendResult{failure: "invalid_request"}
	}
	req.SetBasicAuth(creds.TwilioSID, creds.AccessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(req)
	if err != nil {
		return channelSendResult{failure: "delivery_unknown"}
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == 429:
		return channelSendResult{failure: "rate_limited", retry: true}
	case response.StatusCode >= 500:
		return channelSendResult{failure: "delivery_unknown"}
	case response.StatusCode != http.StatusCreated:
		return channelSendResult{failure: "provider_rejected"}
	}
	var message struct {
		SID    string
		Status string
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&message) != nil || !twilioMessageSID.MatchString(message.SID) {
		return channelSendResult{failure: "delivery_unknown"}
	}
	if message.Status == "failed" || message.Status == "undelivered" {
		return channelSendResult{failure: "provider_rejected"}
	}
	return channelSendResult{receipt: message.SID}
}
