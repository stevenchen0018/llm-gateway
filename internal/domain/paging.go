package domain

// PageQuery is the offset pagination shared by list queries. Limit <= 0
// means "no limit" only where a repository documents it; admin handlers
// always pass a bounded page size.
type PageQuery struct {
	Offset int
	Limit  int
}

// AlertQuery filters the alert list (告警中心).
type AlertQuery struct {
	PageQuery
	DeptID *int64
	Type   string
	Level  string
	Q      string
	// Notified filters on webhook delivery (nil = either).
	Notified *bool
}

// AuditQuery filters the key lifecycle audit trail (审计日志).
type AuditQuery struct {
	PageQuery
	DeptID *int64
	KeyID  *int64
	Action string
	Q      string // operator, detail or key name
}

// BudgetQuery filters budgets (预算管理).
type BudgetQuery struct {
	PageQuery
	DeptID         *int64
	ApprovalStatus string
	Q              string // project, applicant, reason or key name
}

// ApplicationQuery filters applications (应用管理).
type ApplicationQuery struct {
	PageQuery
	DeptID *int64
	Status string
	Q      string // name, owner, description
}
