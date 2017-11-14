package main

import (
	"sort"
	"sync"
	"time"
)

type User struct {
	ID          int64     `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Salt        string    `json:"salt" db:"salt"`
	Password    string    `json:"password" db:"password"`
	DisplayName string    `json:"display_name" db:"display_name"`
	AvatarIcon  string    `json:"avatar_icon" db:"avatar_icon"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

type Channel struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedAt   time.Time `json:"created_at"`

	HaveRead map[int64]int64 `json:"-"`
	Messages []*Message      `json:"-"`
}

func (c *Channel) AddMessage(m *Message) {
	c.Messages = append(c.Messages, m)
	sort.Slice(c.Messages, func(i, j int) bool {
		return c.Messages[i].CreatedAt.Before(c.Messages[j].CreatedAt)
	})
}

func (c *Channel) GetMessagesAfter(id int64) []*Message {
	res := make([]*Message, 0)
	for _, m := range c.Messages {
		if m.ID > id {
			res = append(res, m)
		}
	}
	return res
}

type Message struct {
	ID        int64     `db:"id"`
	ChannelID int64     `db:"channel_id"`
	UserID    int64     `db:"user_id"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`

	User *User
}

type Channels struct {
	sync.Map
}

type Messages struct {
	sync.Map
}

type Dump struct {
	Users    map[int64]*User    `json:"users"`
	Channels map[int64]*Channel `json:"channels"`
	Messages `json:"messages"`
}
