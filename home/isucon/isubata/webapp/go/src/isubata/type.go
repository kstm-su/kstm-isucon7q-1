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

	HaveRead sync.Map   `json:"-"`
	Messages []*Message `json:"-"`

	m sync.RWMutex
}

func (c *Channel) AddMessage(m *Message) {
	c.m.Lock()
	c.Messages = append(c.Messages, m)
	sort.Slice(c.Messages, func(i, j int) bool {
		return c.Messages[i].CreatedAt.Before(c.Messages[j].CreatedAt)
	})
	c.m.Unlock()
}

func (c *Channel) UpdateHaveRead(userID, messageID int64) {
	c.HaveRead.Store(userID, messageID)
}

func (c *Channel) GetHaveRead(userID int64) int64 {
	v, ok := c.HaveRead.Load(userID)
	if !ok {
		return 0
	}
	return v.(int64)
}

func (c *Channel) GetMessagesAfter(id int64) []*Message {
	res := make([]*Message, 0)
	c.m.RLock()
	for _, m := range c.Messages {
		if m.ID > id {
			res = append(res, m)
		}
	}
	c.m.RUnlock()
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

func (c *Channels) Load(id int64) *Channel {
	v, ok := c.Map.Load(id)
	if !ok {
		return nil
	}
	res, _ := v.(*Channel)
	return res
}

func (c *Channels) Range(f func(int64, *Channel) bool) {
	c.Map.Range(func(k, v interface{}) bool {
		id, _ := k.(int64)
		ch, _ := v.(*Channel)
		return f(id, ch)
	})
}

func (c *Channels) Slice() []*Channel {
	res := make([]*Channel, 0)
	c.Range(func(_ int64, ch *Channel) bool {
		res = append(res, ch)
		return true
	})
	return res
}

type Messages struct {
	sync.Map
}

type Dump struct {
	Users    map[int64]*User `json:"users"`
	Channels `json:"channels"`
	Messages `json:"messages"`
}
