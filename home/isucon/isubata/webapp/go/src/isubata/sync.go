package main

import (
	"strconv"

	"github.com/labstack/echo"
)

func syncRegister(c echo.Context) (err error) {
	u := User{}
	if err = c.Bind(&u); err != nil {
		return
	}
	users[u.ID] = &u
	return
}

func syncMessage(c echo.Context) (err error) {
	m := Message{}
	if err = c.Bind(&m); err != nil {
		return
	}
	messages.Store(m.ID, &m)
	channels.Load(m.ChannelID).AddMessage(&m)
	return
}

func syncProfile(c echo.Context) (err error) {
	u := User{}
	if err = c.Bind(&u); err != nil {
		return
	}
	users[u.ID] = &u
	return
}

func syncAddChannel(c echo.Context) (err error) {
	ch := Channel{}
	if err = c.Bind(&ch); err != nil {
		return
	}
	ch.HaveRead = HaveRead{}
	ch.Messages = make([]*Message, 0)
	channels.Store(ch.ID, &ch)
	return
}

func syncHaveRead(c echo.Context) error {
	chanID, err := strconv.ParseInt(c.Param("channel_id"), 10, 64)
	if err != nil {
		return err
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		return err
	}
	messageID, err := strconv.ParseInt(c.Param("message_id"), 10, 64)
	if err != nil {
		return err
	}
	channels.Load(chanID).UpdateHaveRead(userID, messageID)
	return nil
}
