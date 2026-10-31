package isureal

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v5"
)

type createFriendRequestRequest struct {
	RecipientID string `json:"recipientId"`
}

type createFriendRequestResponse struct {
	RelationID string    `json:"relationId"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func createFriendRequestHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var input createFriendRequestRequest
		if err := c.Bind(&input); err != nil {
			return err
		}
		ctx := c.Request().Context()
		actor := GetSession(c).UserID
		if actor == input.RecipientID || input.RecipientID == "" {
			return NewAPIError(http.StatusBadRequest, "invalid_relationship")
		}
		// 手順は sql/008_create_friend_request_function.sql の関数にまとめ、DB とは 1 往復で済ませる（遠い台では 1 往復 100ms かかる）
		var request FriendRequest
		err := h.DB.GetContext(ctx, &request, `SELECT o_id AS id, o_status AS status, o_updated_at AS updated_at FROM isu_create_friend_request($1, $2, $3)`, actor, input.RecipientID, uuid.NewString())
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "IR404":
				return NewAPIError(http.StatusNotFound, "relationship_not_found")
			case "IR409":
				return NewAPIError(http.StatusConflict, "relationship_conflict")
			}
		}
		if err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, createFriendRequestResponse{RelationID: request.ID, Status: request.Status, UpdatedAt: request.UpdatedAt})
	}
}

type transitionFriendRequestRequest struct {
	RequestID string `param:"requestId" json:"-"`
	Action    string `param:"action" json:"-"`
}

type transitionFriendRequestResponse createFriendRequestResponse

func transitionFriendRequestHandler(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var input transitionFriendRequestRequest
		if err := c.Bind(&input); err != nil {
			return err
		}
		var status string
		switch input.Action {
		case "accept":
			status = "accepted"
		case "reject":
			status = "rejected"
		case "cancel":
			status = "cancelled"
		default:
			return NewAPIError(http.StatusForbidden, "relationship_action_forbidden")
		}
		ctx := c.Request().Context()
		actor := GetSession(c).UserID
		// 手順は sql/012_relationship_functions.sql の関数にまとめ、DB とは 1 往復で済ませる（europe1 では承認に 1.19 秒かかっていた）
		var request FriendRequest
		err := h.DB.GetContext(ctx, &request, `SELECT o_id AS id, o_status AS status, o_updated_at AS updated_at FROM isu_transition_friend_request($1, $2, $3)`, actor, input.RequestID, status)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "IR404":
				return NewAPIError(http.StatusNotFound, "relationship_not_found")
			case "IR403":
				return NewAPIError(http.StatusForbidden, "relationship_action_forbidden")
			case "IR409":
				return NewAPIError(http.StatusConflict, "relationship_conflict")
			}
		}
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, transitionFriendRequestResponse{RelationID: request.ID, Status: request.Status, UpdatedAt: request.UpdatedAt})
	}
}
