package domain

import (
	"context"
	"time"
)

// Supplier types (Provider.SupplierType).
const (
	SupplierOfficial   = "official"    // the vendor's own API
	SupplierCloud      = "cloud"       // cloud model platform (百炼, 方舟, 千帆, Vertex, Azure ...)
	SupplierReseller   = "reseller"    // third-party reseller / agent
	SupplierSelfHosted = "self_hosted" // our own GPU cluster
)

// VendorRouting decides how a vendor's traffic is spread over its suppliers
// when no scheduling policy matches the request (the supplier dimension of
// 模型调度).
type VendorRouting string

const (
	VendorRoutingCostFirst VendorRouting = "cost_first"        // cheapest supplier (after discount) first
	VendorRoutingPriority  VendorRouting = "supplier_priority" // by VendorSupplier.Priority
	VendorRoutingWeighted  VendorRouting = "supplier_weighted" // spread by VendorSupplier.Weight
)

// Vendor is a 厂商: the company that makes the models (DeepSeek, 阿里通义,
// OpenAI, Anthropic ...), independent of who sells access to them.
type Vendor struct {
	ID              int64         `json:"id"`
	Code            string        `json:"code"`
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	Website         string        `json:"website"`
	RoutingStrategy VendorRouting `json:"routing_strategy"`
	Status          string        `json:"status"` // active | disabled
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// VendorSupplier links a vendor to one of its suppliers. Disabling the link
// takes that supplier out of routing for this vendor's models only.
type VendorSupplier struct {
	ID         int64     `json:"id"`
	VendorID   int64     `json:"vendor_id"`
	ProviderID int64     `json:"provider_id"`
	Priority   int       `json:"priority"`
	Weight     int       `json:"weight"`
	Status     string    `json:"status"` // active | disabled
	Remark     string    `json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (s *VendorSupplier) Active() bool { return s == nil || s.Status != "disabled" }

type VendorRepository interface {
	Create(ctx context.Context, v *Vendor) error
	Update(ctx context.Context, v *Vendor) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*Vendor, error)
	List(ctx context.Context) ([]*Vendor, error)

	ListSuppliers(ctx context.Context, vendorID *int64) ([]*VendorSupplier, error)
	GetSupplier(ctx context.Context, id int64) (*VendorSupplier, error)
	CreateSupplier(ctx context.Context, s *VendorSupplier) error
	UpdateSupplier(ctx context.Context, s *VendorSupplier) error
	DeleteSupplier(ctx context.Context, id int64) error
}
