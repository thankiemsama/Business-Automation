package service

import (
	"encoding/json"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const skillRoutingKind = "skill_routing"

type SkillGroup struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Members  []string `json:"members"`
	Capacity int      `json:"capacity"`
}

type SkillRoute struct {
	Channel  string `json:"channel"`
	Tag      string `json:"tag"`
	Priority string `json:"priority"`
	Primary  string `json:"primary"`
	Overflow string `json:"overflow"`
}

type SkillRouting struct {
	Enabled bool         `json:"enabled"`
	Groups  []SkillGroup `json:"groups"`
	Rules   []SkillRoute `json:"rules"`
}

func loadSkillRouting(store core.Reader, tenant string) (SkillRouting, error) {
	config := SkillRouting{Groups: []SkillGroup{}, Rules: []SkillRoute{}}
	if record, found := store.Record(tenant, tokenHash("skill-routing:"+tenant), skillRoutingKind); found {
		err := json.Unmarshal([]byte(record.Content), &config)
		return config, err
	}
	return config, nil
}

func (o *Operations) SkillRouting(user core.User) (SkillRouting, error) {
	if user.Role != core.RoleOwner {
		return SkillRouting{}, core.Fail(403, "Chỉ owner quản lý định tuyến")
	}
	var config SkillRouting
	err := o.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		var err error
		config, err = loadSkillRouting(store, user.TenantID)
		return err
	})
	return config, err
}

func (o *Operations) SaveSkillRouting(user core.User, config SkillRouting) (SkillRouting, error) {
	if user.Role != core.RoleOwner {
		return config, core.Fail(403, "Chỉ owner quản lý định tuyến")
	}
	if len(config.Groups) > 30 || len(config.Rules) > 50 || (config.Enabled && len(config.Rules) == 0) {
		return config, core.Fail(400, "Cấu hình cần tối đa 30 nhóm, 50 quy tắc và ít nhất một quy tắc khi bật")
	}
	err := o.repo.updateAsTenant(user.TenantID, user.ID, func(store core.Writer) error {
		groups := map[string]bool{}
		for i := range config.Groups {
			group := &config.Groups[i]
			group.Name = strings.TrimSpace(group.Name)
			if group.Key == "" || len(group.Key) > 80 || groups[group.Key] || group.Name == "" ||
				len([]rune(group.Name)) > 80 || group.Capacity < 1 || group.Capacity > 100 || len(group.Members) == 0 || len(group.Members) > 100 {
				return core.Fail(400, "Tên, thành viên hoặc tải tối đa của nhóm không hợp lệ")
			}
			groups[group.Key] = true
			seen := map[string]bool{}
			for _, id := range group.Members {
				member, found := store.UserByID(id)
				if !found || member.Disabled || member.TenantID != user.TenantID || seen[id] {
					return core.Fail(400, "Thành viên nhóm phải là tài khoản đang hoạt động trong doanh nghiệp")
				}
				seen[id] = true
			}
		}
		for i := range config.Rules {
			rule := &config.Rules[i]
			rule.Tag = strings.TrimSpace(rule.Tag)
			if rule.Tag != "" {
				tags, err := normalizeTags([]string{rule.Tag})
				if err != nil {
					return err
				}
				rule.Tag = tags[0]
			}
			if !groups[rule.Primary] || rule.Overflow == rule.Primary ||
				(rule.Overflow != "" && !groups[rule.Overflow]) ||
				(rule.Channel != "" && !contains(core.PublicChannels, rule.Channel) &&
					!contains([]string{core.ChannelWeb, core.ChannelEmail, core.ChannelPhone}, rule.Channel)) ||
				(rule.Priority != "" && !contains([]string{core.PriorityLow, core.PriorityNormal, core.PriorityHigh, core.PriorityUrgent}, rule.Priority)) {
				return core.Fail(400, "Quy tắc có kênh, độ ưu tiên hoặc nhóm không hợp lệ")
			}
		}
		payload, err := json.Marshal(config)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: tokenHash("skill-routing:" + user.TenantID), TenantID: user.TenantID,
			Kind: skillRoutingKind, Content: string(payload), UpdatedAt: o.clock.Now().UTC()})
		return nil
	})
	return config, err
}

func routeNewConversation(store core.Writer, conversation *core.Record, now time.Time) {
	config, err := loadSkillRouting(store, conversation.TenantID)
	if err != nil || !config.Enabled {
		return
	}
	customer, _ := store.Record(conversation.TenantID, conversation.CustomerID, core.KindCustomers)
	priority := conversation.Priority
	if priority == "" {
		priority = core.PriorityNormal
	}
	for _, rule := range config.Rules {
		if (rule.Channel != "" && rule.Channel != conversation.Channel) ||
			(rule.Priority != "" && rule.Priority != priority) ||
			(rule.Tag != "" && !contains(conversation.Tags, rule.Tag) && !contains(customer.Tags, rule.Tag)) {
			continue
		}
		load := map[string]int{}
		recent := map[string]time.Time{}
		for _, thread := range store.Records(conversation.TenantID, core.KindConversations) {
			if thread.AssigneeID != "" && thread.Status != core.StatusClosed {
				load[thread.AssigneeID]++
			}
		}
		for _, key := range []string{rule.Primary, rule.Overflow} {
			for _, group := range config.Groups {
				if key == "" || group.Key != key {
					continue
				}
				selected := ""
				for _, id := range group.Members {
					member, allowed := store.UserByID(id)
					if !allowed || member.Disabled || member.TenantID != conversation.TenantID || load[id] >= group.Capacity {
						continue
					}
					marker, _ := store.Record(conversation.TenantID, routingMarkerID(conversation.TenantID, id), "skill_routing_rotation")
					recent[id] = marker.UpdatedAt
					if selected == "" || load[id] < load[selected] ||
						(load[id] == load[selected] && (recent[id].Before(recent[selected]) || recent[id].Equal(recent[selected]) && id < selected)) {
						selected = id
					}
				}
				if selected != "" {
					conversation.AssigneeID = selected
					store.PutRecord(core.Record{ID: routingMarkerID(conversation.TenantID, selected), TenantID: conversation.TenantID,
						Kind: "skill_routing_rotation", UpdatedAt: now})
					return
				}
			}
		}
		return
	}
}

func routingMarkerID(tenant, user string) string {
	return tokenHash("skill-routing:" + tenant + ":" + user)
}
