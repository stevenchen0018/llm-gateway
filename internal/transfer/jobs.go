package transfer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Job is one recorded import (commit) or export.
type Job struct {
	ID           int64        `json:"id" gorm:"column:id;primaryKey"`
	Direction    string       `json:"direction" gorm:"column:direction"`
	Entity       string       `json:"entity" gorm:"column:entity"`
	EntityTitle  string       `json:"entity_title" gorm:"column:entity_title"`
	Format       string       `json:"format" gorm:"column:format"`
	FileName     string       `json:"file_name" gorm:"column:file_name"`
	Operator     string       `json:"operator" gorm:"column:operator"`
	OperatorID   *int64       `json:"operator_id,omitempty" gorm:"column:operator_id"`
	DepartmentID *int64       `json:"department_id,omitempty" gorm:"column:department_id"`
	Status       string       `json:"status" gorm:"column:status"`
	Total        int          `json:"total" gorm:"column:total"`
	Created      int          `json:"created" gorm:"column:created"`
	Updated      int          `json:"updated" gorm:"column:updated"`
	Skipped      int          `json:"skipped" gorm:"column:skipped"`
	Failed       int          `json:"failed" gorm:"column:failed"`
	Message      string       `json:"message" gorm:"column:message"`
	ErrorsJSON   string       `json:"-" gorm:"column:errors;type:jsonb"`
	Errors       []FieldError `json:"errors" gorm:"-"`
	CreatedAt    time.Time    `json:"created_at" gorm:"column:created_at"`
}

func (Job) TableName() string { return "data_transfers" }

// Jobs stores transfer records.
type Jobs struct{ db *gorm.DB }

func NewJobs(db *gorm.DB) *Jobs { return &Jobs{db: db} }

func (j *Jobs) Record(ctx context.Context, job *Job) error {
	if len(job.Errors) > 200 {
		job.Errors = job.Errors[:200]
	}
	b, _ := json.Marshal(job.Errors)
	if job.Errors == nil {
		b = []byte("[]")
	}
	job.ErrorsJSON = string(b)
	if err := j.db.WithContext(ctx).Create(job).Error; err != nil {
		return fmt.Errorf("record transfer job: %w", err)
	}
	return nil
}

type JobQuery struct {
	DeptID    *int64
	Direction string
	Entity    string
	Offset    int
	Limit     int
}

func (j *Jobs) Page(ctx context.Context, q JobQuery) ([]*Job, int64, error) {
	tx := j.db.WithContext(ctx).Model(&Job{})
	if q.DeptID != nil {
		tx = tx.Where("department_id = ?", *q.DeptID)
	}
	if q.Direction != "" {
		tx = tx.Where("direction = ?", q.Direction)
	}
	if q.Entity != "" {
		tx = tx.Where("entity = ?", q.Entity)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count transfer jobs: %w", err)
	}
	var rows []*Job
	if err := tx.Order("created_at desc, id desc").Offset(q.Offset).Limit(q.Limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list transfer jobs: %w", err)
	}
	for _, r := range rows {
		_ = json.Unmarshal([]byte(r.ErrorsJSON), &r.Errors)
		if r.Errors == nil {
			r.Errors = []FieldError{}
		}
	}
	return rows, total, nil
}
