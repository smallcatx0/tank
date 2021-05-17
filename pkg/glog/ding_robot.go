package glog

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type DingAlarm struct {
	webHook   string
	secret    string
	sign      string
	timestamp string
	Msg       *DingMsg
}

type DingMsg struct {
	Msgtype  string       `json:"msgtype"`
	Text     DingBodyText `json:"text"`
	Markdown DingBodyMd   `json:"markdown"`
	At       DingBodyAt   `json:"at"`
}

type DingBodyText struct {
	Content string `json:"content"`
}

type DingBodyMd struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type DingBodyAt struct {
	AtMobiles []string `json:"atMobiles"`
	AtUserIds []string `json:"atUserIds"`
	IsAtAll   bool     `json:"isAtAll"`
}

func DingAlarmNew(webHook, secret string) *DingAlarm {
	d := &DingAlarm{
		webHook: webHook,
		secret:  secret,
		Msg:     &DingMsg{},
	}
	return d
}

func (d *DingAlarm) signature() string {
	now := time.Now().Unix() * 1000
	d.timestamp = strconv.FormatInt(now, 10)
	h := hmac.New(sha256.New, []byte(d.secret))
	h.Write([]byte(d.timestamp + "\n" + d.secret))
	sign := base64.URLEncoding.EncodeToString(h.Sum(nil))
	sign = url.PathEscape(sign)
	sign = strings.Replace(sign, "-", "%2B", -1)
	sign = strings.Replace(sign, "_", "%2F", -1)
	d.sign = sign
	return sign
}

// TODO: 开携程
func (d *DingAlarm) Send() error {

	return d.SendMsg(d.Msg)
}

func (d *DingAlarm) SendMsg(msg *DingMsg) error {
	sign := d.signature()
	url := d.webHook + "&timestamp=" + d.timestamp + "&sign=" + sign
	body, _ := json.Marshal(msg)
	resp, err := new(http.Client).Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	res, _ := ioutil.ReadAll(resp.Body)
	ress := make(map[string]interface{})
	json.Unmarshal(res, &ress)
	errcd, ok := ress["errcode"].(float64)
	if ok && errcd == 0 {
		return nil
	}
	return errors.New(string(res))
}

func (d *DingAlarm) Text(con string) *DingAlarm {
	d.Msg.Msgtype = "text"
	d.Msg.Text = DingBodyText{
		Content: con,
	}
	return d
}

func (d *DingAlarm) Markdown(title, md string) *DingAlarm {
	d.Msg.Msgtype = "markdown"
	d.Msg.Markdown = DingBodyMd{
		Title: title,
		Text:  md,
	}
	return d
}

func (d *DingAlarm) AtPhones(phone ...string) *DingAlarm {
	d.Msg.At = DingBodyAt{
		AtMobiles: phone,
	}
	return d
}

func (d *DingAlarm) AtUsers(id ...string) *DingAlarm {
	d.Msg.At = DingBodyAt{
		AtUserIds: id,
	}
	return d
}

func (d *DingAlarm) AtAll() *DingAlarm {
	d.Msg.At = DingBodyAt{
		IsAtAll: true,
	}
	return d
}

func (d *DingAlarm) SendMd(title, content string) error {
	msg := DingMsg{
		Msgtype: "markdown",
		Markdown: DingBodyMd{
			Title: title,
			Text:  content,
		},
	}
	return d.SendMsg(&msg)
}
