package httpapi

import (
	"errors"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func (a *API) registerRenewalRoutes(r *gin.Engine, admin *gin.RouterGroup) {
	r.GET("/api/me/renewal-plan", a.auth(), a.renewalPlan)
	r.GET("/api/me/renewals", a.auth(), func(c *gin.Context) { a.renewalsFor(c, current(c).ID) })
	r.POST("/api/me/renewals", a.auth(), a.requestRenewal)
	r.POST("/api/me/renewals/:id/cancel", a.auth(), func(c *gin.Context) { a.resolveRenewal(c, "cancel", true) })
	admin.GET("/renewals", a.allRenewals)
	admin.POST("/renewals/:id/approve", func(c *gin.Context) { a.resolveRenewal(c, "approve", false) })
	admin.POST("/renewals/:id/reject", func(c *gin.Context) { a.resolveRenewal(c, "reject", false) })
	admin.GET("/users/:id/renewals", func(c *gin.Context) {
		if id, ok := positiveID(c); ok {
			a.renewalsFor(c, id)
		}
	})
}
func (a *API) renewalPlan(c *gin.Context) {
	a.jsonQuery(c, `SELECT jsonb_build_object('plan',CASE WHEN p.id IS NULL THEN NULL ELSE jsonb_build_object('id',p.id,'name',p.name,'quota_bytes',p.quota_bytes,'duration_days',p.duration_days,'price_cents',p.price_cents) END,'expires_at',u.expires_at) FROM users u LEFT JOIN plans p ON p.id=u.plan_id WHERE u.id=$1`, current(c).ID)
}
func (a *API) renewalsFor(c *gin.Context, id int64) {
	a.jsonQuery(c, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT * FROM renewal_requests WHERE user_id=$1 ORDER BY id DESC LIMIT 2000)t`, id)
}
func (a *API) allRenewals(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1000000 {
		fail(c, 400, "invalid page")
		return
	}
	a.jsonQuery(c, `SELECT jsonb_build_object('total',(SELECT count(*) FROM renewal_requests),'rows',COALESCE((SELECT jsonb_agg(to_jsonb(t)) FROM (SELECT r.*,u.email FROM renewal_requests r JOIN users u ON u.id=r.user_id ORDER BY (r.status='pending') DESC,r.id DESC LIMIT 20 OFFSET $1)t),'[]'::jsonb))`, (page-1)*20)
}
func (a *API) requestRenewal(c *gin.Context) {
	var in struct {
		RequestID string `json:"request_id"`
	}
	if !decode(c, &in) {
		return
	}
	r, err := a.s.RequestRenewal(c.Request.Context(), current(c).ID, in.RequestID)
	if a.renewalError(c, err) {
		return
	}
	c.JSON(200, r)
}
func (a *API) resolveRenewal(c *gin.Context, action string, ownerOnly bool) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	r, err := a.s.ResolveRenewal(c.Request.Context(), id, current(c).ID, action, ownerOnly)
	if a.renewalError(c, err) {
		return
	}
	c.JSON(200, r)
}
func (a *API) renewalError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrInvalid):
		fail(c, 400, "请提供有效的续期请求编号。")
	case errors.Is(err, store.ErrConflict):
		fail(c, 409, "已有待处理申请，或套餐/账号状态已变化。请刷新后重试；无到期时间的账号无需续期。")
	case errors.Is(err, pgx.ErrNoRows):
		fail(c, 404, "续期订单或套餐不存在。")
	default:
		dbFailure(c, err)
	}
	return true
}
