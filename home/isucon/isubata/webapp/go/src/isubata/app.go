package main

import (
	"bytes"
	"compress/gzip"
	crand "crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/middleware"
	"github.com/parnurzeal/gorequest"
)

const (
	avatarMaxBytes = 1 * 1024 * 1024
)

var (
	db            *sqlx.DB
	ErrBadReqeust = echo.NewHTTPError(http.StatusBadRequest)
	redisClient   *redis.Client
	other1        string
	other2        string

	users    map[int64]*User
	channels Channels
	messages Messages
)

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

type Renderer struct {
	templates *template.Template
}

func (r *Renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

func init() {
	seedBuf := make([]byte, 8)
	crand.Read(seedBuf)
	rand.Seed(int64(binary.LittleEndian.Uint64(seedBuf)))
	other1 = os.Getenv("ISUBATA_OTHER_HOST1")
	other2 = os.Getenv("ISUBATA_OTHER_HOST2")

	users = make(map[int64]*User)
	channels = Channels{}
	messages = Messages{}

	db_host := os.Getenv("ISUBATA_DB_HOST")
	if db_host == "" {
		db_host = "127.0.0.1"
	}
	db_port := os.Getenv("ISUBATA_DB_PORT")
	if db_port == "" {
		db_port = "3306"
	}
	db_user := os.Getenv("ISUBATA_DB_USER")
	if db_user == "" {
		db_user = "root"
	}
	db_password := os.Getenv("ISUBATA_DB_PASSWORD")
	if db_password != "" {
		db_password = ":" + db_password
	}

	dsn := fmt.Sprintf("%s%s@tcp(%s:%s)/isubata?parseTime=true&loc=Local&charset=utf8mb4",
		db_user, db_password, db_host, db_port)

	log.Printf("Connecting to db: %q", dsn)
	db, _ = sqlx.Connect("mysql", dsn)
	for {
		err := db.Ping()
		if err == nil {
			break
		}
		log.Println(err)
		time.Sleep(time.Second * 3)
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr: "app3:6379",
	})

	ub, err := redisClient.Get("users").Bytes()
	if err != nil {
		log.Fatal("failed to restore users: ", err)
	}
	if err := gob.NewDecoder(bytes.NewBuffer(ub)).Decode(&users); err != nil {
		log.Fatal("failed to decode users: ", err)
	}
	log.Println("restored users")

	ch := make(map[int64]*Channel)
	cb, err := redisClient.Get("channels").Bytes()
	if err != nil {
		log.Fatal("failed to restore channels: ", err)
	}
	if err := gob.NewDecoder(bytes.NewBuffer(cb)).Decode(&ch); err != nil {
		log.Fatal("failed to decode channels: ", err)
	}
	for k, v := range ch {
		channels.Store(k, v)
	}
	log.Println("restored channels")

	mh := make(map[int64]*Message)
	mb, err := redisClient.Get("messages").Bytes()
	if err != nil {
		log.Fatal("failed to restore messages: ", err)
	}
	if err := gob.NewDecoder(bytes.NewBuffer(mb)).Decode(&mh); err != nil {
		log.Fatal("failed to decode messages: ", err)
	}
	for k, v := range mh {
		messages.Store(k, v)
	}
	log.Println("restored messages")

	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(10 * time.Minute)
	log.Printf("Succeeded to connect db.")

	if os.Getenv("ISUBATA_SERVER_ID") == "03" {
		go func() {
			for {
				time.Sleep(time.Second * 180)

				userBuf := bytes.Buffer{}
				userEnc := gob.NewEncoder(&userBuf)
				if err := userEnc.Encode(users); err != nil {
					log.Fatal("failed to save user:", err)
				}
				redisClient.Set("users", userBuf.Bytes(), 0)
				log.Println("saved user")

				channelBuf := bytes.Buffer{}
				channelEnc := gob.NewEncoder(&channelBuf)
				if err := channelEnc.Encode(channels.Hash()); err != nil {
					log.Fatal("failed to save channel:", err)
				}
				redisClient.Set("channels", channelBuf.Bytes(), 0)
				log.Println("saved channel")

				messageBuf := bytes.Buffer{}
				messageEnc := gob.NewEncoder(&messageBuf)
				if err := messageEnc.Encode(messages.Hash()); err != nil {
					log.Fatal("failed to save message:", err)
				}
				redisClient.Set("messages", messageBuf.Bytes(), 0)
				log.Println("saved message")
			}
		}()
	}
}

func getUser(userID int64) (*User, error) {
	return users[userID], nil
}

func addMessage(channelID, userID int64, content string) (int64, error) {
	id, err := redisClient.Incr("message").Result()
	if err != nil {
		return 0, err
	}
	m := &Message{
		ID:        id,
		ChannelID: channelID,
		UserID:    userID,
		Content:   content,
		CreatedAt: time.Now(),
		User:      users[userID],
	}
	messages.Store(id, m)
	channels.Load(channelID).AddMessage(m)
	gorequest.New().Post("http://" + other1 + "/sync/message").Send(m).End()
	gorequest.New().Post("http://" + other2 + "/sync/message").Send(m).End()
	return id, nil
}

type IMessage struct {
	ID        int64     `db:"id"`
	ChannelID int64     `db:"channel_id"`
	UserID    int64     `db:"user_id"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`

	Name        string `json:"name" db:"name"`
	DisplayName string `json:"display_name" db:"display_name"`
	AvatarIcon  string `json:"avatar_icon" db:"avatar_icon"`
}

func queryMessages(chanID, lastID int64) []*Message {
	m := channels.Load(chanID).GetMessagesAfter(lastID)
	if len(m) > 100 {
		m = m[len(m)-100:]
	}
	return m
}

func sessUserID(c echo.Context) int64 {
	sess, _ := session.Get("session", c)
	var userID int64
	if x, ok := sess.Values["user_id"]; ok {
		userID, _ = x.(int64)
	}
	return userID
}

func sessSetUserID(c echo.Context, id int64) {
	sess, _ := session.Get("session", c)
	sess.Options = &sessions.Options{
		HttpOnly: true,
		MaxAge:   360000,
	}
	sess.Values["user_id"] = id
	sess.Save(c.Request(), c.Response())
}

func ensureLogin(c echo.Context) (*User, error) {
	var user *User
	var err error

	userID := sessUserID(c)
	if userID == 0 {
		goto redirect
	}

	user, err = getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		sess, _ := session.Get("session", c)
		delete(sess.Values, "user_id")
		sess.Save(c.Request(), c.Response())
		goto redirect
	}
	return user, nil

redirect:
	c.Redirect(http.StatusSeeOther, "/login")
	return nil, nil
}

const LettersAndDigits = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(n int) string {
	b := make([]byte, n)
	z := len(LettersAndDigits)

	for i := 0; i < n; i++ {
		b[i] = LettersAndDigits[rand.Intn(z)]
	}
	return string(b)
}

func register(name, password string) (int64, error) {
	id, err := redisClient.Incr("user").Result()
	if err != nil {
		return 0, err
	}
	salt := randomString(20)
	digest := fmt.Sprintf("%x", sha1.Sum([]byte(salt+password)))

	users[id] = &User{
		ID:          id,
		Name:        name,
		Salt:        salt,
		Password:    digest,
		DisplayName: name,
		AvatarIcon:  "default.png",
		CreatedAt:   time.Now(),
	}
	return id, nil
}

// request handlers

func getIndex(c echo.Context) error {
	userID := sessUserID(c)
	if userID != 0 {
		return c.Redirect(http.StatusSeeOther, "/channel/1")
	}

	return c.Render(http.StatusOK, "index", map[string]interface{}{
		"ChannelID": nil,
	})
}

func getChannel(c echo.Context) error {
	user, err := ensureLogin(c)
	if user == nil {
		return err
	}
	cID, err := strconv.Atoi(c.Param("channel_id"))
	if err != nil {
		return err
	}
	ch := channels.Load(int64(cID))
	return c.Render(http.StatusOK, "channel", map[string]interface{}{
		"ChannelID":   cID,
		"Channels":    channels.Slice(),
		"User":        user,
		"Description": ch.Description,
	})
}

func getRegister(c echo.Context) error {
	return c.Render(http.StatusOK, "register", map[string]interface{}{
		"ChannelID": 0,
		"Channels":  []Channel{},
		"User":      nil,
	})
}

func postRegister(c echo.Context) error {
	name := c.FormValue("name")
	pw := c.FormValue("password")
	if name == "" || pw == "" {
		return ErrBadReqeust
	}
	for _, u := range users {
		if u.Name == name {
			return c.NoContent(http.StatusConflict)
		}
	}
	userID, _ := register(name, pw)
	gorequest.New().Post("http://" + other1 + "/sync/register").Send(users[userID]).End()
	gorequest.New().Post("http://" + other2 + "/sync/register").Send(users[userID]).End()
	sessSetUserID(c, userID)
	return c.Redirect(http.StatusSeeOther, "/")
}

func getLogin(c echo.Context) error {
	return c.Render(http.StatusOK, "login", map[string]interface{}{
		"ChannelID": 0,
		"Channels":  []Channel{},
		"User":      nil,
	})
}

func postLogin(c echo.Context) error {
	name := c.FormValue("name")
	pw := c.FormValue("password")
	if name == "" || pw == "" {
		return ErrBadReqeust
	}

	var user *User
	for _, u := range users {
		if u.Name == name {
			user = u
			break
		}
	}
	if user == nil {
		return echo.ErrForbidden
	}

	digest := fmt.Sprintf("%x", sha1.Sum([]byte(user.Salt+pw)))
	if digest != user.Password {
		return echo.ErrForbidden
	}
	sessSetUserID(c, user.ID)
	return c.Redirect(http.StatusSeeOther, "/")
}

func getLogout(c echo.Context) error {
	sess, _ := session.Get("session", c)
	delete(sess.Values, "user_id")
	sess.Save(c.Request(), c.Response())
	return c.Redirect(http.StatusSeeOther, "/")
}

func postMessage(c echo.Context) error {
	user, err := ensureLogin(c)
	if user == nil {
		return err
	}

	message := c.FormValue("message")
	if message == "" {
		return echo.ErrForbidden
	}

	var chanID int64
	if x, err := strconv.Atoi(c.FormValue("channel_id")); err != nil {
		return echo.ErrForbidden
	} else {
		chanID = int64(x)
	}

	_, err = addMessage(chanID, user.ID, message)
	if err != nil {
		return err
	}
	return c.NoContent(204)
}

func jsonifyMessage(m *Message) (map[string]interface{}, error) {
	u, ok := users[m.UserID]
	if !ok {
		return nil, fmt.Errorf("nil user: %d of %+v", m.UserID, m)
	}

	r := make(map[string]interface{})
	r["id"] = m.ID
	r["user"] = struct {
		AvatarIcon  string `json:"avatar_icon"`
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
	}{
		AvatarIcon:  u.AvatarIcon,
		DisplayName: u.DisplayName,
		Name:        u.Name,
	}
	r["date"] = m.CreatedAt.Format("2006/01/02 15:04:05")
	r["content"] = m.Content
	return r, nil
}

func getMessage(c echo.Context) error {
	userID := sessUserID(c)
	if userID == 0 {
		return c.NoContent(http.StatusForbidden)
	}

	chanID, err := strconv.ParseInt(c.QueryParam("channel_id"), 10, 64)
	if err != nil {
		return err
	}
	lastID, err := strconv.ParseInt(c.QueryParam("last_message_id"), 10, 64)
	if err != nil {
		return err
	}

	ms := queryMessages(chanID, lastID)

	response := make([]map[string]interface{}, 0)
	//for i := len(messages) - 1; i >= 0; i-- {
	for i, m := range ms {
		//m := messages[i]
		if m == nil {
			fmt.Printf("nil message: %d", i)
			continue
		}
		r, err := jsonifyMessage(m)
		if err != nil {
			return err
		}
		response = append(response, r)
	}

	if len(ms) > 0 {
		channels.Load(chanID).UpdateHaveRead(userID, ms[len(ms)-1].ID)
		gorequest.New().Get(fmt.Sprintf("http://%s/sync/haveread/%d/%d/%d", other1, chanID, userID, ms[len(ms)-1].ID)).End()
		gorequest.New().Get(fmt.Sprintf("http://%s/sync/haveread/%d/%d/%d", other2, chanID, userID, ms[len(ms)-1].ID)).End()
	}

	return c.JSON(http.StatusOK, response)
}

func queryChannels() ([]int64, error) {
	res := make([]int64, 0)
	channels.Range(func(id int64, _ *Channel) bool {
		res = append(res, id)
		return true
	})
	return res, nil
}

func fetchUnread(c echo.Context) error {
	userID := sessUserID(c)
	if userID == 0 {
		return c.NoContent(http.StatusForbidden)
	}

	time.Sleep(time.Millisecond * 7000)

	resp := []map[string]interface{}{}

	channels.Range(func(chID int64, ch *Channel) bool {
		lastID := ch.GetHaveRead(userID)
		var cnt int64
		ch.m.RLock()
		for _, m := range ch.Messages {
			if m.ID > lastID {
				cnt++
			}
		}
		ch.m.RUnlock()
		r := map[string]interface{}{
			"channel_id": chID,
			"unread":     cnt,
		}
		resp = append(resp, r)
		return true
	})

	return c.JSON(http.StatusOK, resp)
}

func getHistory(c echo.Context) error {
	chID, err := strconv.ParseInt(c.Param("channel_id"), 10, 64)
	if err != nil || chID <= 0 {
		return ErrBadReqeust
	}

	user, err := ensureLogin(c)
	if user == nil {
		return err
	}

	var page int64
	pageStr := c.QueryParam("page")
	if pageStr == "" {
		page = 1
	} else {
		page, err = strconv.ParseInt(pageStr, 10, 64)
		if err != nil || page < 1 {
			return ErrBadReqeust
		}
	}

	const N = 20
	ch := channels.Load(int64(chID))
	ch.m.RLock()
	ms := ch.Messages[:]
	ch.m.RUnlock()
	cnt := int64(len(ms))
	maxPage := int64(cnt+N-1) / N
	if maxPage == 0 {
		maxPage = 1
	}
	if page > maxPage {
		return ErrBadReqeust
	}

	rev := make([]*Message, len(ms))
	for i, m := range ms {
		rev[len(ms)-i-1] = m
	}
	msgs := rev[:]
	begin := min((page-1)*N, int64(len(ms)))
	end := min(page*N, int64(len(ms)))
	if end > 0 {
		fmt.Println("page: ", len(ms), begin, end)
		msgs = rev[begin:end]
	}

	mjson := make([]map[string]interface{}, 0)
	for i := len(msgs) - 1; i >= 0; i-- {
		r, err := jsonifyMessage(msgs[i])
		if err != nil {
			return err
		}
		mjson = append(mjson, r)
	}

	return c.Render(http.StatusOK, "history", map[string]interface{}{
		"ChannelID": chID,
		"Channels":  channels.Slice(),
		"Messages":  mjson,
		"MaxPage":   maxPage,
		"Page":      page,
		"User":      user,
	})
}

func getProfile(c echo.Context) error {
	self, err := ensureLogin(c)
	if self == nil {
		return err
	}

	//channels := []ChannelInfo{}
	//err = db.Select(&channels, "SELECT * FROM channel ORDER BY id")
	//if err != nil {
	//	return err
	//}

	userName := c.Param("user_name")
	var other *User
	for _, u := range users {
		if u.Name == userName {
			other = u
		}
	}
	//err = db.Get(&other, "SELECT * FROM user WHERE name = ?", userName)
	//if err == sql.ErrNoRows {
	if other == nil {
		return echo.ErrNotFound
	}
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "profile", map[string]interface{}{
		"ChannelID":   0,
		"Channels":    channels.Slice(),
		"User":        self,
		"Other":       other,
		"SelfProfile": self.ID == other.ID,
	})
}

func getAddChannel(c echo.Context) error {
	self, err := ensureLogin(c)
	if self == nil {
		return err
	}

	//channels := []ChannelInfo{}
	//err = db.Select(&channels, "SELECT * FROM channel ORDER BY id")
	//if err != nil {
	//	return err
	//}

	return c.Render(http.StatusOK, "add_channel", map[string]interface{}{
		"ChannelID": 0,
		"Channels":  channels.Slice(),
		"User":      self,
	})
}

func postAddChannel(c echo.Context) error {
	self, err := ensureLogin(c)
	if self == nil {
		return err
	}

	name := c.FormValue("name")
	desc := c.FormValue("description")
	if name == "" || desc == "" {
		return ErrBadReqeust
	}

	lastID, err := redisClient.Incr("channel").Result()
	//res, err := db.Exec(
	//	"INSERT INTO channel (name, description, updated_at, created_at) VALUES (?, ?, NOW(), NOW())",
	//	name, desc)
	if err != nil {
		return err
	}
	now := time.Now()
	ch := &Channel{
		ID:          lastID,
		Name:        name,
		Description: desc,
		UpdatedAt:   now,
		CreatedAt:   now,
		HaveRead:    HaveRead{},
		Messages:    make([]*Message, 0),
	}
	channels.Store(lastID, ch)
	gorequest.New().Post("http://" + other1 + "/sync/channel").Send(ch).End()
	gorequest.New().Post("http://" + other2 + "/sync/channel").Send(ch).End()
	//lastID, _ := res.LastInsertId()
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/channel/%v", lastID))
}

func makeGzip(body []byte) ([]byte, error) {
	var b bytes.Buffer
	err := func() error {
		gw := gzip.NewWriter(&b)
		defer gw.Close()

		if _, err := gw.Write(body); err != nil {
			return err
		}
		return nil
	}()
	return b.Bytes(), err
}

var (
	fileNameMutex   sync.Mutex
	fileNameMutexId = 1
)

func fileName() string {
	fileNameMutex.Lock()
	var ret = fileNameMutexId
	fileNameMutexId++
	fileNameMutex.Unlock()
	return strconv.Itoa(ret)
}

func postProfile(c echo.Context) error {
	self, err := ensureLogin(c)
	if self == nil {
		return err
	}

	avatarName := ""
	var avatarData []byte

	if fh, err := c.FormFile("avatar_icon"); err == http.ErrMissingFile {
		// no file upload
	} else if err != nil {
		return err
	} else {
		dotPos := strings.LastIndexByte(fh.Filename, '.')
		if dotPos < 0 {
			return ErrBadReqeust
		}
		ext := fh.Filename[dotPos:]
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif":
			break
		default:
			return ErrBadReqeust
		}

		file, err := fh.Open()
		if err != nil {
			return err
		}
		avatarData, _ = ioutil.ReadAll(file)
		file.Close()

		if len(avatarData) > avatarMaxBytes {
			return ErrBadReqeust
		}

		//avatarName = fmt.Sprintf("%x%s", sha1.Sum(avatarData), ext)
		avatarName = fmt.Sprintf("%x%s", fileName(), ext)
	}

	if avatarName != "" && len(avatarData) > 0 {
		/*
			_, err := db.Exec("INSERT INTO image (name, data) VALUES (?, ?)", avatarName, avatarData)
		*/
		avatarDataGzip, err := makeGzip(avatarData)
		if err != nil {
			return err
		}
		ioutil.WriteFile("/home/isucon/isubata/webapp/public/icons/"+avatarName+".gz", avatarDataGzip, os.ModePerm)
		users[self.ID].AvatarIcon = os.Getenv("ISUBATA_SERVER_ID") + "/" + avatarName
	}

	if name := c.FormValue("display_name"); name != "" {
		users[self.ID].DisplayName = name
	}

	gorequest.New().Post("http://" + other1 + "/sync/profile").Send(users[self.ID]).End()
	gorequest.New().Post("http://" + other2 + "/sync/profile").Send(users[self.ID]).End()

	return c.Redirect(http.StatusSeeOther, "/")
}

func tAdd(a, b int64) int64 {
	return a + b
}

func tRange(a, b int64) []int64 {
	r := make([]int64, b-a+1)
	for i := int64(0); i <= (b - a); i++ {
		r[i] = a + i
	}
	return r
}

func main() {
	e := echo.New()
	funcs := template.FuncMap{
		"add":    tAdd,
		"xrange": tRange,
	}
	e.Renderer = &Renderer{
		templates: template.Must(template.New("").Funcs(funcs).ParseGlob("views/*.html")),
	}
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("secretonymoris"))))
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "request:\"${method} ${uri}\" status:${status} latency:${latency} (${latency_human}) bytes:${bytes_out}\n",
	}))
	e.Use(middleware.Static("../public"))

	e.GET("/initialize", getInitialize)
	e.GET("/", getIndex)
	e.GET("/register", getRegister)
	e.POST("/register", postRegister)
	e.GET("/login", getLogin)
	e.POST("/login", postLogin)
	e.GET("/logout", getLogout)

	e.GET("/channel/:channel_id", getChannel)
	e.GET("/message", getMessage)
	e.POST("/message", postMessage)
	e.GET("/fetch", fetchUnread)
	e.GET("/history/:channel_id", getHistory)

	e.GET("/profile/:user_name", getProfile)
	e.POST("/profile", postProfile)

	e.GET("add_channel", getAddChannel)
	e.POST("add_channel", postAddChannel)

	e.POST("/sync/register", syncRegister)
	e.POST("/sync/message", syncMessage)
	e.POST("/sync/profile", syncProfile)
	e.POST("/sync/channel", syncAddChannel)
	e.GET("/sync/haveread/:channel_id/:user_id/:message_id", syncHaveRead)
	e.GET("/sync/initialize", syncInitialize)

	e.GET("/dump", dump)

	e.Start(":5000")
}
