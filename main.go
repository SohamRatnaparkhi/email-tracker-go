package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gopkg.in/gomail.v2"
)

var emailDialer *gomail.Dialer
var rdb *redis.Client

var err error = godotenv.Load()

// fmt.Println(err)

func main() {
	_, err := os.Stat(".env")
	if err == nil {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}
	PORT := os.Getenv("PORT")

	if PORT == "" {
		PORT = "8080"
	}

	initEmailDialer()
	initRedisClient()

	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.GET("/pixel/:tracking_id", handleTrackingPixel)
	r.GET("/status/:tracking_id", handleTrackingStatus)
	r.POST("/send", handleEmailRequest)

	r.Run(":" + PORT) // listen and serve on 0.0.0.0:8080
}

func initEmailDialer() error {
	smtpPort, err := strconv.Atoi(os.Getenv("SMTP_PORT"))
	if err != nil {
		return err
	}

	emailDialer = gomail.NewDialer(
		os.Getenv("SMTP_HOST"),
		smtpPort,
		os.Getenv("SMTP_USERNAME"),
		os.Getenv("SMTP_PASSWORD"),
	)
	return nil
}

func initRedisClient() error {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	opt, err := redis.ParseURL(redisAddr)
	if err != nil {
		return err
	}

	rdb = redis.NewClient(opt)

	ctx := context.Background()
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		return err
	}

	return nil
}

type Receiver struct {
	Email       string `json:"email"`
	TrackingId  string `json:"tracking_id"`
	WantToTrack bool   `json:"want_to_track"`
	Type        string `json:"type"` // cc, bcc, to
}

type Recipients struct {
	Receivers []Receiver `json:"receivers"`
	From      string     `json:"from"`
}

type EmailBody struct {
	HtmlTemplate string                 `json:"html_template"`
	Subject      string                 `json:"subject"`
	Parameters   map[string]interface{} `json:"parameters"` // email mapped to params where params are key value pairs. Key = variable name and value = value
}

type TrackingObject struct {
	Email      string    `json:"email"`
	Count      int       `json:"count"`
	LastOpened time.Time `json:"last_opened"`
}

type EmailTrackerRequest struct {
	Recipients Recipients `json:"recipients"`
	EmailBody  EmailBody  `json:"email_body"`
}

func handleEmailRequest(c *gin.Context) {
	var emailTrackerRequest EmailTrackerRequest
	err := c.BindJSON(&emailTrackerRequest)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid request",
		})
		return
	}

	statusMap := sendEmails(emailTrackerRequest.Recipients, emailTrackerRequest.EmailBody)
	c.JSON(200, gin.H{
		"status": statusMap,
	})
}

func handleTrackingPixel(c *gin.Context) {
	trackingId := c.Param("tracking_id")
	ctx := context.Background()

	key := "tracking:" + trackingId
	var trackingObj TrackingObject
	data, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		c.Status(404)
		return
	}

	if err := json.Unmarshal(data, &trackingObj); err != nil {
		c.Status(500)
		return
	}

	trackingObj.Count++
	trackingObj.LastOpened = time.Now()

	updatedData, _ := json.Marshal(trackingObj)
	rdb.Set(ctx, key, updatedData, getTrackingExpiration())

	// Return transparent pixel
	pixelData, _ := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
	)
	c.Data(200, "image/png", pixelData)
}

func handleTrackingStatus(c *gin.Context) {
	trackingId := c.Param("tracking_id")
	ctx := context.Background()

	key := "tracking:" + trackingId
	var trackingObj TrackingObject
	data, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		c.JSON(404, gin.H{
			"error": "Tracking ID not found",
		})
		return
	}

	if err := json.Unmarshal(data, &trackingObj); err != nil {
		c.JSON(500, gin.H{
			"error": "Internal server error",
		})
		return
	}

	c.JSON(200, gin.H{
		"email":       trackingObj.Email,
		"count":       trackingObj.Count,
		"last_opened": trackingObj.LastOpened,
	})
}

func sendEmails(recipients Recipients, emailBody EmailBody) map[string]interface{} {
	statusMap := make(map[string]interface{})
	emailToTemplateMap := setHtml(emailBody)

	for _, receiver := range recipients.Receivers {
		m := gomail.NewMessage()

		m.SetHeader("From", recipients.From)

		m.SetHeader("Subject", emailBody.Subject)

		switch receiver.Type {
		case "to":
			m.SetHeader("To", receiver.Email)
		case "cc":
			m.SetHeader("Cc", receiver.Email)
		case "bcc":
			m.SetHeader("Bcc", receiver.Email)
		default:
			statusMap[receiver.Email] = "Failed: invalid recipient type"
			continue
		}

		if receiver.WantToTrack {
			receiver.TrackingId = setTrackingId(receiver.Email, receiver.TrackingId, true)
		}

		template, ok := emailToTemplateMap[receiver.Email]
		if !ok {
			statusMap[receiver.Email] = "Failed: no template found for recipient"
			continue
		}

		pixelHtml := fmt.Sprintf(
			`<img src="%s/pixel/%s" alt="" width="1" height="1" style="display:none"/>`,
			os.Getenv("TRACKING_DOMAIN"),
			receiver.TrackingId,
		)
		template = strings.Replace(template.(string), "</body>", pixelHtml+"</body>", 1)

		m.SetBody("text/html", template.(string))

		if err := emailDialer.DialAndSend(m); err != nil {
			statusMap[receiver.Email] = "Failed:tracking_id:" + receiver.TrackingId + " error:" + err.Error()
		} else {
			statusMap[receiver.Email] = "Success:tracking_id:" + receiver.TrackingId
		}
	}
	return statusMap
}

func setHtml(emailBody EmailBody) map[string]interface{} {
	template := emailBody.HtmlTemplate
	emailToTemplateMap := make(map[string]interface{})

	for key, value := range emailBody.Parameters {
		email := key
		for k, v := range value.(map[string]interface{}) {
			template = strings.ReplaceAll(template, "{{ "+k+" }}", v.(string))
		}
		emailToTemplateMap[email] = template
	}
	return emailToTemplateMap
}

func setTrackingId(email string, trackingId string, generateId bool) string {
	ctx := context.Background()
	if generateId {
		trackingId = uuid.New().String()
	}

	trackingObject := TrackingObject{
		Email:      email,
		Count:      0,
		LastOpened: time.Now(),
	}

	trackingObjectJson, _ := json.Marshal(trackingObject)
	key := "tracking:" + trackingId
	rdb.Set(ctx, key, trackingObjectJson, getTrackingExpiration())

	return trackingId
}

func getTrackingExpiration() time.Duration {
	expirationStr := os.Getenv("TRACKING_ID_EXPIRATION")
	expiration, err := strconv.Atoi(expirationStr)
	if err != nil || expirationStr == "" {
		expiration = 7 * 24 * 60 * 60 // default to 7 day in seconds
	}
	return time.Duration(expiration) * time.Second
}
