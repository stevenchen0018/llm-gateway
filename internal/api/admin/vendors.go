package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// VendorHandler serves 厂商管理 (vendors and which suppliers supply them) and
// the supplier dimension of 模型调度 (per-supplier priority / weight / status).
type VendorHandler struct {
	vendors *service.VendorService
}

func NewVendorHandler(v *service.VendorService) *VendorHandler { return &VendorHandler{vendors: v} }

func (h *VendorHandler) List(c *gin.Context) {
	list, err := h.vendors.List(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondList(c, filterList(list, func(v service.VendorView) bool {
		return eqParam(c, "status", v.Status) && matchQ(c, v.Name, v.Code, v.Description)
	}))
}

func (h *VendorHandler) Create(c *gin.Context) {
	var req dto.VendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	v := &domain.Vendor{Code: req.Code, Name: req.Name, Description: req.Description, Website: req.Website,
		RoutingStrategy: domain.VendorRouting(req.RoutingStrategy), Status: req.Status}
	if err := h.vendors.Create(c.Request.Context(), v); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, v)
}

func (h *VendorHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.VendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	v, err := h.vendors.Get(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	v.Name, v.Description, v.Website = req.Name, req.Description, req.Website
	if req.RoutingStrategy != "" {
		v.RoutingStrategy = domain.VendorRouting(req.RoutingStrategy)
	}
	if req.Status != "" {
		v.Status = req.Status
	}
	if err := h.vendors.Update(c.Request.Context(), v); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, v)
}

func (h *VendorHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.vendors.Delete(c.Request.Context(), id); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"deleted": id})
}

func (h *VendorHandler) AddSupplier(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.VendorSupplierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	l := &domain.VendorSupplier{VendorID: id, ProviderID: req.ProviderID, Priority: 100, Weight: 100, Status: req.Status}
	if req.Priority != nil {
		l.Priority = *req.Priority
	}
	if req.Weight != nil {
		l.Weight = *req.Weight
	}
	if req.Remark != nil {
		l.Remark = *req.Remark
	}
	if err := h.vendors.AddSupplier(c.Request.Context(), l); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, l)
}

func (h *VendorHandler) UpdateSupplier(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.VendorSupplierRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	l, err := h.vendors.GetSupplier(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if req.Priority != nil {
		l.Priority = *req.Priority
	}
	if req.Weight != nil {
		l.Weight = *req.Weight
	}
	if req.Status != "" {
		l.Status = req.Status
	}
	if req.Remark != nil {
		l.Remark = *req.Remark
	}
	if err := h.vendors.UpdateSupplier(c.Request.Context(), l); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, l)
}

func (h *VendorHandler) RemoveSupplier(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.vendors.RemoveSupplier(c.Request.Context(), id); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"deleted": id})
}
