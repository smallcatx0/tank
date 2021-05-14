package glog

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

type DingAlarm struct {
	webHook   string
	secret    string
	sign      string
	timestamp string
}

type DingBody struct {
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
	}
	return d
}

func (d *DingAlarm) Send(msg DingBody) error {
	d.signature()
	url := d.webHook + "&timestamp=" + d.timestamp + "&sign=" + d.sign
	body, _ := json.Marshal(msg)
	resp, err := new(http.Client).Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	res, _ := ioutil.ReadAll(resp.Body)
	log.Print(string(res))
	return nil
}

func (d DingAlarm) signature() {
	now := time.Now().Unix() * 1000
	d.timestamp = strconv.FormatInt(now, 10)
	h := hmac.New(sha256.New, []byte(d.secret))
	h.Write([]byte(d.timestamp + "\n" + d.secret))
	sign := base64.URLEncoding.EncodeToString(h.Sum(nil))
	// sign = url.PathEscape(sign)
	// sign = strings.Replace(sign, "-", "%2B", -1)
	// sign = strings.Replace(sign, "_", "%2F", -1)
	d.sign = sign
}
