package service

import "businessautomation/internal/core"

type DashboardService struct{ repo core.Repository }

func NewDashboard(repo core.Repository) *DashboardService { return &DashboardService{repo: repo} }

func (s *DashboardService) Dashboard(user core.User) (map[string]int, error) {
	counts := map[string]int{
		core.KindCustomers:     0,
		core.KindConversations: 0,
		core.KindTickets:       0,
		core.KindKnowledge:     0,
		core.KindAutomations:   0,
		core.KindMessages:      0,
		"open_tickets":         0,
	}
	err := s.repo.View(func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, "") {
			if _, isDashboardKind := counts[record.Kind]; record.TenantID == user.TenantID && isDashboardKind {
				counts[record.Kind]++
				if record.Kind == core.KindTickets && record.Status != core.StatusResolved {
					counts["open_tickets"]++
				}
			}
		}
		return nil
	})
	return counts, err
}
