package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var errorCodes = map[int]string{
	http.StatusBadRequest:          "BAD_REQUEST",
	http.StatusUnauthorized:        "UNAUTHORIZED",
	http.StatusForbidden:           "FORBIDDEN",
	http.StatusNotFound:            "NOT_FOUND",
	http.StatusConflict:            "CONFLICT",
	http.StatusTooManyRequests:     "RATE_LIMITED",
	http.StatusInternalServerError: "INTERNAL_ERROR",
	http.StatusServiceUnavailable:  "SERVICE_UNAVAILABLE",
}

func HTTPErrorHandler(err error, c echo.Context) {
	code := http.StatusInternalServerError
	msg := "internal error"

	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
		code = he.Code
		switch m := he.Message.(type) {
		case nil:
		case string:
			msg = m
		default:
			msg = fmt.Sprintf("%v", m)
		}
	}

	codeStr, ok := errorCodes[code]
	if !ok {
		codeStr = "UNKNOWN"
	}

	if code >= http.StatusInternalServerError {
		slog.Error("request failed",
			"request_id", c.Get(echo.HeaderXRequestID),
			"method", c.Request().Method,
			"path", c.Request().URL.Path,
			"status", code,
			"error", err,
		)
		msg = "internal error"
	}

	if !c.Response().Committed {
		c.JSON(code, ErrorResponse{
			Code:    codeStr,
			Message: msg,
		})
	}
}
