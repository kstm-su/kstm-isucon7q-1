package main

import (
	"bytes"
	"encoding/gob"
	"os"
	"strconv"

	"github.com/labstack/echo"
)

func getInitialize(c echo.Context) error {
	db.MustExec("DELETE FROM user WHERE id > 1000")
	db.MustExec("DELETE FROM image WHERE id > 1001")
	db.MustExec("DELETE FROM channel WHERE id > 10")
	db.MustExec("DELETE FROM message WHERE id > 10000")
	db.MustExec("DELETE FROM haveread")

	os.Rename("/home/isucon/work", "/home/isucon/isubata/webapp/public/icons")

	err := resetRedis()
	if err != nil {
		return err
	}

	return c.String(204, "")
}

func resetRedis() error {
	redisClient.FlushDB()

	messages, err := getAllMessages()
	if err != nil {
		return err
	}
	for c, m := range messages {
		if err := redisClient.LPush("m"+strconv.FormatInt(c, 10), m...).Err(); err != nil {
			return err
		}
	}
	return nil
}

func getAllMessages() ([][]byte, error) {
	messages := make(map[int64][][]byte)
	rows, err := db.Query("SELECT id, channel_id, user_id, content, created_at FROM message")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m := Message{}
		rows.Scan(&m.ID, &m.ChannelID, &m.UserID, &m.Content, &m.CreatedAt)
		if _, exist := messages[m.ChannelID]; !exist {
			messages[m.ChannelID] = make([][]byte, 0, 1)
		}
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(m); err != nil {
			return nil, err
		}
		messages[m.ChannelID] = append(messages[m.ChannelID], buf.Bytes())
	}
	return messages, nil
}
