package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"businessautomation/internal/core"
)

var externalField = regexp.MustCompile("^[A-Za-z][A-Za-z0-9_]{0,79}$")

func (a *Admin) CRMOptions(tenant string) map[string]bool {
	allowed := os.Getenv("NEXA_CRM_TENANT") == tenant && tenant != ""
	return map[string]bool{
		"hubspot":    allowed && os.Getenv("HUBSPOT_TOKEN") != "",
		"salesforce": allowed && os.Getenv("SALESFORCE_TOKEN") != "" && os.Getenv("SALESFORCE_ORIGIN") != "" && externalField.MatchString(os.Getenv("SALESFORCE_EXTERNAL_ID_FIELD")),
		"odoo":       allowed && os.Getenv("ODOO_API_KEY") != "" && os.Getenv("ODOO_ORIGIN") != "" && externalField.MatchString(os.Getenv("ODOO_EXTERNAL_ID_FIELD")),
	}
}
func (a *Admin) SyncCRM(ctx context.Context, user core.User, provider, id string) error {
	if !a.CRMOptions(user.TenantID)[provider] {
		return core.Fail(503, "CRM chưa cấu hình cho doanh nghiệp")
	}
	a.crmMu.Lock()
	defer a.crmMu.Unlock()
	var customer core.Record
	if err := a.repo.View(func(store core.Reader) error {
		var err error
		customer, err = findOwnedRecord(store, user, id, core.KindCustomers)
		return err
	}); err != nil {
		return err
	}
	var err error
	switch provider {
	case "hubspot":
		if customer.Email == "" {
			return core.Fail(400, "Cần email để đồng bộ HubSpot")
		}
		properties := map[string]string{"email": customer.Email, "firstname": customer.Name, "phone": customer.Phone}
		err = crmRequest(ctx, "POST", "https://api.hubapi.com/crm/v3/objects/contacts/batch/upsert", os.Getenv("HUBSPOT_TOKEN"), "", map[string]any{"inputs": []map[string]any{{"id": customer.Email, "idProperty": "email", "properties": properties}}}, nil)
	case "salesforce":
		base, valid := crmOrigin(os.Getenv("SALESFORCE_ORIGIN"))
		if !valid {
			return core.Fail(503, "Địa chỉ Salesforce không hợp lệ")
		}
		path := base + "/services/data/v61.0/sobjects/Contact/" + os.Getenv("SALESFORCE_EXTERNAL_ID_FIELD") + "/" + url.PathEscape(customer.ID)
		err = crmRequest(ctx, "PATCH", path, os.Getenv("SALESFORCE_TOKEN"), "", map[string]string{"LastName": customer.Name, "Email": customer.Email, "Phone": customer.Phone}, nil)
	case "odoo":
		base, valid := crmOrigin(os.Getenv("ODOO_ORIGIN"))
		if !valid {
			return core.Fail(503, "Địa chỉ Odoo không hợp lệ")
		}
		base += "/json/2/res.partner/"
		key, db, field := os.Getenv("ODOO_API_KEY"), os.Getenv("ODOO_DATABASE"), os.Getenv("ODOO_EXTERNAL_ID_FIELD")
		var matches []struct{ ID int }
		if err := crmRequest(ctx, "POST", base+"search_read", key, db, map[string]any{"domain": [][]string{{field, "=", customer.ID}}, "fields": []string{"id"}, "limit": 2}, &matches); err != nil {
			return err
		}
		if len(matches) > 1 {
			return core.Fail(409, "Odoo có nhiều liên hệ cùng mã Nexa")
		}
		values := map[string]string{field: customer.ID, "name": customer.Name, "email": customer.Email, "phone": customer.Phone}
		if len(matches) == 0 {
			err = crmRequest(ctx, "POST", base+"create", key, db, map[string]any{"vals_list": []map[string]string{values}}, nil)
		} else {
			err = crmRequest(ctx, "POST", base+"write", key, db, map[string]any{"ids": []int{matches[0].ID}, "vals": values}, nil)
		}
	}
	if err != nil {
		return err
	}
	return a.repo.updateAs(user.ID, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: tokenHash("crm:" + user.TenantID + ":" + provider + ":" + id), TenantID: user.TenantID, Kind: "crm_sync", CustomerID: id, Name: provider, UpdatedAt: a.clock.Now()})
		return nil
	})
}
func crmOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	return strings.TrimRight(parsed.String(), "/"), true
}
func crmRequest(ctx context.Context, method, target, key, db string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	if db != "" {
		req.Header.Set("X-Odoo-Database", db)
	}
	response, err := providerClient.Do(req)
	if err != nil {
		return core.Fail(502, "Không kết nối được CRM")
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 || response.StatusCode == 207 {
		return core.Fail(502, "CRM từ chối đồng bộ")
	}
	if out != nil && json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out) != nil {
		return core.Fail(502, "CRM trả kết quả không hợp lệ")
	}
	return nil
}
