package handler

import (
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

	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		if he.Message != nil {
			msg = he.Message.(string)
		}
	}

	codeStr, ok := errorCodes[code]
	if !ok {
		codeStr = "UNKNOWN"
	}

	if !c.Response().Committed {
		c.JSON(code, ErrorResponse{
			Code:    codeStr,
			Message: msg,
		})
	}
}