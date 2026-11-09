package services

import (
	"fmt"
	"log"
	"net/smtp"
	"net/url"
	"strings"

	"thinker/internal/config"
)

type MailerService struct {
	cfg *config.BaseConfig
}

func NewMailerService(cfg *config.BaseConfig) *MailerService {
	return &MailerService{cfg: cfg}
}

func (m *MailerService) SendVerificationEmail(toEmail, rawToken string) error {
	verifyURL := fmt.Sprintf("%s/auth/verify-email?token=%s", m.cfg.AppBaseURL, url.QueryEscape(rawToken))
	subject := "Thinker - Confirma tu correo electrónico (válido por 15 minutos)"
	body := fmt.Sprintf(
		"Hola,\n\nPara activar tu cuenta en Thinker, confirma tu correo electrónico haciendo clic en el siguiente enlace de un solo uso (expira en 15 minutos):\n\n%s\n\nSi no solicitaste esta cuenta, puedes ignorar este mensaje.\n",
		verifyURL,
	)

	if m.cfg.Debug {
		log.Printf("\n============================================================\n[DEBUG MAILER] Confirmación de correo para %s\nEnlace (expira en 15m, un solo uso): %s\n============================================================\n", toEmail, verifyURL)
	}

	return m.send(toEmail, subject, body)
}

func (m *MailerService) SendPasswordResetEmail(toEmail, rawToken string) error {
	resetURL := fmt.Sprintf("%s/auth/reset-password?token=%s", m.cfg.AppBaseURL, url.QueryEscape(rawToken))
	subject := "Thinker - Recuperación de contraseña (válido por 15 minutos)"
	body := fmt.Sprintf(
		"Hola,\n\nRecibimos una solicitud para restablecer tu contraseña en Thinker. Usa el siguiente enlace de un solo uso (expira en 15 minutos):\n\n%s\n\nSi no realizaste esta solicitud, ignora este correo; tu contraseña seguirá siendo la misma.\n",
		resetURL,
	)

	if m.cfg.Debug {
		log.Printf("\n============================================================\n[DEBUG MAILER] Recuperación de contraseña para %s\nEnlace (expira en 15m, un solo uso): %s\n============================================================\n", toEmail, resetURL)
	}

	return m.send(toEmail, subject, body)
}

func (m *MailerService) send(toEmail, subject, body string) error {
	if strings.TrimSpace(m.cfg.SMTPHost) == "" {
		return nil
	}

	addr := m.cfg.SMTPHost + ":" + m.cfg.SMTPPort
	var smtpAuth smtp.Auth
	if m.cfg.SMTPUsername != "" {
		smtpAuth = smtp.PlainAuth("", m.cfg.SMTPUsername, m.cfg.SMTPPassword, m.cfg.SMTPHost)
	}

	msg := strings.Join([]string{
		"From: " + m.cfg.SMTPFrom,
		"To: " + toEmail,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"",
		body,
	}, "\r\n")

	return smtp.SendMail(addr, smtpAuth, m.cfg.SMTPFrom, []string{toEmail}, []byte(msg))
}
