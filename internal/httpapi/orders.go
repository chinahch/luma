package httpapi

import (
	"errors"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (a *API) registerBusinessRoutes(r *gin.Engine, admin *gin.RouterGroup) {
	admin.PATCH("/nodes/:id/active", a.setNodeActive)
	admin.DELETE("/nodes/:id", a.deleteNode)
	admin.GET("/users/:id/orders", a.userOrders)
	admin.POST("/users/:id/orders", a.allocateOrder)
	r.GET("/api/me/orders", a.auth(), func(c *gin.Context) { a.ordersFor(c, current(c).ID) })
}
func (a *API) ordersFor(c *gin.Context, id int64) {
	a.jsonQuery(c, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT * FROM orders WHERE user_id=$1 ORDER BY id DESC LIMIT 2000)t`, id)
}
func (a *API) userOrders(c *gin.Context) {
	id, ok := positiveID(c)
	if ok {
		a.ordersFor(c, id)
	}
}
func (a *API) allocateOrder(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in struct {
		PlanID    int64  `json:"plan_id"`
		RequestID string `json:"request_id"`
	}
	if !decode(c, &in) {
		return
	}
	o, err := a.s.AllocateOrder(c.Request.Context(), id, current(c).ID, in.PlanID, in.RequestID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalid):
			fail(c, 400, "plan_id and request_id (1..128 bytes) required")
		case errors.Is(err, store.ErrConflict):
			fail(c, 409, "request_id belongs to a different plan")
		case errors.Is(err, pgx.ErrNoRows):
			fail(c, 404, "user or plan not found")
		default:
			dbFailure(c, err)
		}
		return
	}
	c.JSON(200, o)
}
