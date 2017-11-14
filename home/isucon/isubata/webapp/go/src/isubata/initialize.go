package main

import (
	"github.com/labstack/echo"
	"github.com/parnurzeal/gorequest"
)

func getInitialize(c echo.Context) error {
	db.MustExec("DELETE FROM user WHERE id > 1000")
	db.MustExec("DELETE FROM image WHERE id > 1001")
	db.MustExec("DELETE FROM channel WHERE id > 10")
	db.MustExec("DELETE FROM message WHERE id > 10000")
	db.MustExec("DELETE FROM haveread")

	redisClient.FlushDB()
	redisClient.Set("user", 1000, 0)
	redisClient.Set("channel", 10, 0)
	redisClient.Set("message", 10000, 0)

	gorequest.New().Get("http://" + other + "/sync/initialize").End()
	return syncInitialize(c)
}

func syncInitialize(c echo.Context) error {
	err := resetRedis()
	if err != nil {
		return err
	}

	return c.String(204, "")
}

func resetRedis() error {
	users = make(map[int64]*User)
	channels = make(map[int64]*Channel)
	messages = make(map[int64]*Message)

	if err := initializeUsers(); err != nil {
		return err
	}
	if err := initializeChannels(); err != nil {
		return err
	}
	if err := initializeMessages(); err != nil {
		return err
	}
	return nil
}

func initializeUsers() error {
	rows, err := db.Query("SELECT id, name, salt, password, display_name, avatar_icon, created_at FROM user")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Salt, &u.Password, &u.DisplayName, &u.AvatarIcon, &u.CreatedAt); err != nil {
			return err
		}
		users[u.ID] = &u
	}
	return nil
}

func initializeChannels() error {
	rows, err := db.Query("SELECT id, name, description, updated_at, created_at FROM channel")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.UpdatedAt, &c.CreatedAt); err != nil {
			return err
		}
		channels[c.ID] = &c
	}
	return nil
}

func initializeMessages() error {
	rows, err := db.Query("SELECT id, channel_id, user_id, content, created_at FROM message")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.UserID, &m.Content, &m.CreatedAt); err != nil {
			return err
		}
		m.User = users[m.UserID]
		messages[m.ID] = &m
		channels[m.ChannelID].HaveRead = make(map[int64]int64)
		channels[m.ChannelID].Messages = append(channels[m.ChannelID].Messages, &m)
	}
	return nil
}
