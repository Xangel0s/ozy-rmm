package notifier

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host       string
	Port       string
	User       string
	Password   string
	From       string
	Recipients []string
}

func SendAlert(cfg SMTPConfig, subject, body string) {
	if cfg.Host == "" || len(cfg.Recipients) == 0 {
		return
	}

	msg := buildMessage(cfg.From, cfg.Recipients, subject, body)
	addr := cfg.Host + ":" + cfg.Port

	auth := smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)

	for retry := 0; retry < 2; retry++ {
		err := smtp.SendMail(addr, auth, cfg.From, cfg.Recipients, []byte(msg))
		if err == nil {
			return
		}
		log.Printf("email: send failed (attempt %d): %v", retry+1, err)
		time.Sleep(2 * time.Second)
	}
}

func buildMessage(from string, to []string, subject, body string) string {
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n",
		from, strings.Join(to, ", "), subject)
	return headers + body
}
