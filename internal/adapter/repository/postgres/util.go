package postgres

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

func nowNano() int64 { return time.Now().UnixNano() }

// deptKeyIDs is a subquery selecting the IDs of every API key owned by the
// department (application keys and employees' personal keys).
func deptKeyIDs(db *gorm.DB, deptID int64) *gorm.DB {
	return db.Table("api_keys").Select("api_keys.id").Where("api_keys.department_id = ?", deptID)
}

// escapeLike escapes LIKE/ILIKE wildcards so user input matches literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
