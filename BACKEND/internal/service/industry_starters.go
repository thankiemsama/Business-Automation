package service

import (
	"encoding/json"

	"businessautomation/internal/core"
)

type industryStarter struct {
	subject string
	guide   string
	keyword string
	survey  string
}

var industryStarters = map[string]industryStarter{
	"ecommerce": {
		subject: "Kiểm tra đơn hàng và đổi trả",
		guide:   "Xác minh mã đơn qua kênh hỗ trợ riêng. Kiểm tra trạng thái giao và điều kiện đổi trả đã công bố trước khi xác nhận thời hạn với khách.",
		keyword: "đổi trả",
		survey:  "Bạn hài lòng thế nào với cách chúng tôi hỗ trợ đơn hàng của bạn?",
	},
	"food_beverage": {
		subject: "Xử lý phản hồi về đơn món",
		guide:   "Ghi nhận mã đơn, thời điểm và chi nhánh. Chuyển phản ánh về an toàn thực phẩm cho quản lý ca và hướng dẫn khách theo quy trình đã được phê duyệt.",
		keyword: "sai món",
		survey:  "Bạn hài lòng thế nào với cách chúng tôi hỗ trợ đơn món của bạn?",
	},
	"saas": {
		subject: "Tiếp nhận sự cố phần mềm",
		guide:   "Ghi nhận tính năng, thời điểm, thông báo lỗi và mức độ ảnh hưởng. Yêu cầu ảnh chụp đã ẩn dữ liệu nhạy cảm. Không yêu cầu mật khẩu hoặc token.",
		keyword: "không đăng nhập được",
		survey:  "Bạn hài lòng thế nào với việc hỗ trợ sự cố phần mềm?",
	},
	"education": {
		subject: "Hỗ trợ truy cập lớp học",
		guide:   "Xác minh khóa học, buổi học và tài khoản qua kênh riêng. Kiểm tra lịch và quyền truy cập. Trao đổi dữ liệu học viên qua kênh đã được phê duyệt.",
		keyword: "không vào lớp",
		survey:  "Bạn hài lòng thế nào với việc hỗ trợ truy cập lớp học?",
	},
	"logistics": {
		subject: "Tra cứu vận đơn và giao chậm",
		guide:   "Đối chiếu mã vận đơn với sự kiện giao nhận gần nhất. Xác minh với bộ phận điều phối trước khi đưa ra thời gian giao dự kiến.",
		keyword: "giao chậm",
		survey:  "Bạn hài lòng thế nào với việc hỗ trợ tra cứu vận đơn?",
	},
}

func (s *AuthService) seedIndustryStarter(store core.Writer, user core.User, starter industryStarter) error {
	now := s.clock.Now().UTC()
	article := core.Record{
		ID: s.ids.NewID(), TenantID: user.TenantID, Kind: core.KindKnowledge,
		Subject: starter.subject, Content: starter.guide, CreatedAt: now, UpdatedAt: now,
	}
	if err := (KnowledgePolicy{}).Validate(store, user, &article); err != nil {
		return err
	}
	store.PutRecord(article)

	store.PutRecord(core.Record{
		ID: s.ids.NewID(), TenantID: user.TenantID, Kind: "templates",
		Subject: "Mẫu CSAT — " + starter.subject, Content: starter.survey + " Bạn có thể đánh giá từ 1 đến 5 và góp ý thêm.",
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	})

	rules := []WorkflowRule{
		{
			ID: s.ids.NewID(), Name: "Phân loại — " + starter.subject, Trigger: "message_received",
			Conditions: WorkflowConditions{Contains: starter.keyword},
			Actions:    []WorkflowAction{{Type: "notify_owner"}},
		},
		{
			ID: s.ids.NewID(), Name: "SLA — Báo quản trị khi phiếu ưu tiên cao quá hạn", Trigger: "sla_breached",
			Conditions: WorkflowConditions{Priority: core.PriorityHigh},
			Actions:    []WorkflowAction{{Type: "notify_owner"}},
		},
	}
	for _, rule := range rules {
		data, err := json.Marshal(rule)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{
			ID: rule.ID, TenantID: user.TenantID, Kind: workflowRulesKind,
			Content: string(data), CreatedAt: now, UpdatedAt: now,
		})
	}
	return nil
}
