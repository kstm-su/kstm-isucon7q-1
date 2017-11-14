package main

import (
	"net/http"

	"github.com/labstack/echo"
)

func dump(c echo.Context) error {
	return c.JSON(http.StatusOK, &Dump{Users: users, Channels: channels, Messages: messages})
}
