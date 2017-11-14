package main

import (
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

	rows, err := db.Query("SELECT id, content FROM message")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var content string
		rows.Scan(&id, &content)
		err := redisClient.Set(strconv.FormatInt(id, 10), content, 0).Err()
		if err != nil {
			return err
		}
	}
	return nil
}
