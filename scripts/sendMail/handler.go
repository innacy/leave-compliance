package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zinscky/log"
)

const emailTemplate = `
<!DOCTYPE html>
<html>
<head>
  <style>
    body {
      font-family: Arial, sans-serif;
      color: #333333;
      margin: 0;
      padding: 0;
      background-color: #f5f7fa;
    }
    .container {
      max-width: 600px;
      margin: 20px auto;
      background: #ffffff;
      padding: 24px;
      border-radius: 8px;
      border: 1px solid #e5e7eb;
    }
    .header {
      color: #1a73e8;
      margin-top: 0;
      margin-bottom: 16px;
    }
    .dates {
      background: #f8f9fa;
      border-left: 4px solid #1a73e8;
      padding: 12px 16px;
      margin: 16px 0;
      border-radius: 4px;
    }
    .dates ul {
      margin: 0;
      padding-left: 20px;
    }
    .footer {
      margin-top: 24px;
      font-size: 13px;
      color: #666666;
      border-top: 1px solid #eeeeee;
      padding-top: 16px;
    }
  </style>
</head>
<body>
  <div class="container">
    <h3 class="header">Leave Application Reminder</h3>
    <p>Hi {{.EmpName}},</p>
    <p>Our attendance records indicate that you were absent on the following date(s) but have <strong>not submitted a leave request</strong> in the greytHR portal:</p>
    <div class="dates">
      {{.Dates}}
    </div>
    <p>Please submit your leave application at your earliest convenience to avoid LOP. If you have already submitted your leave application, kindly ignore this reminder.</p>
    <div class="footer">
      <p>Regards,<br><strong>HR Team</strong></p>
      <p><em>This is an automated reminder. Please do not reply to this email.</em></p>
    </div>
  </div>
</body>
</html>
`

type AbsenteeReport struct {
	Date      string           `json:"date"`
	Absentees []AbsenteeDetail `json:"absentees"`
}

type AbsenteeDetail struct {
	EmpCode      string `json:"empCode"`
	EmpName      string `json:"empName,omitempty"`
	Email        string `json:"email,omitempty"`
	LeaveApplied bool   `json:"leaveApplied"`
}

type EmailRequest struct {
	Message struct {
		Subject string `json:"subject"`
		Body    struct {
			ContentType string `json:"contentType"`
			Content     string `json:"content"`
		} `json:"body"`
		ToRecipients []Recipient `json:"toRecipients"`
		CcRecipients []Recipient `json:"ccRecipients,omitempty"`
	} `json:"message"`
	SaveToSentItems bool `json:"saveToSentItems"`
}

type Recipient struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}

func Execute(event string, config map[string]string, vars map[string]string, log *log.Logger) (string, error) {
	log.Info("inside send mail handler")

	accessToken := vars["microsoftToken"]
	senderEmail := config["senderEmail"]

	if accessToken == "" {
		return "", fmt.Errorf("microsoftToken not found in vars")
	}
	if senderEmail == "" {
		return "", fmt.Errorf("senderEmail not found in config")
	}

	var ccEmails []string
	if ccRaw := strings.TrimSpace(config["ccEmails"]); ccRaw != "" {
		for _, addr := range strings.Split(ccRaw, ",") {
			if trimmed := strings.TrimSpace(addr); trimmed != "" {
				ccEmails = append(ccEmails, trimmed)
			}
		}
	}

	var reports []AbsenteeReport
	if err := json.Unmarshal([]byte(event), &reports); err != nil {
		return "", fmt.Errorf("unmarshal absentee reports: %w", err)
	}

	unapplied := collectUnappliedAbsentees(reports)
	log.Info("found %d employees with unapplied leaves", len(unapplied))

	type mailResult struct {
		EmpName string   `json:"empName"`
		Email   string   `json:"email"`
		Dates   []string `json:"dates"`
		Status  string   `json:"status"`
		Error   string   `json:"error,omitempty"`
	}

	var results []mailResult
	var sent, failed int

	for email, info := range unapplied {
		htmlContent := buildEmailContent(info.name, info.dates)

		err := sendMail(accessToken, senderEmail, email, ccEmails, htmlContent)
		if err != nil {
			log.Error("failed to send mail to %s (%s): %v", info.name, email, err)
			results = append(results, mailResult{
				EmpName: info.name,
				Email:   email,
				Dates:   info.dates,
				Status:  "failed",
				Error:   err.Error(),
			})
			failed++
			continue
		}
		log.Info("sent reminder to %s (%s) for dates: %s", info.name, email, strings.Join(info.dates, ", "))
		results = append(results, mailResult{
			EmpName: info.name,
			Email:   email,
			Dates:   info.dates,
			Status:  "sent",
		})
		sent++
	}

	output, _ := json.Marshal(map[string]interface{}{
		"sent":    sent,
		"failed":  failed,
		"total":   len(unapplied),
		"details": results,
	})
	return string(output), nil
}

type absenteeInfo struct {
	name  string
	dates []string
}

func collectUnappliedAbsentees(reports []AbsenteeReport) map[string]*absenteeInfo {
	unapplied := map[string]*absenteeInfo{}

	for _, report := range reports {
		for _, absentee := range report.Absentees {
			if absentee.LeaveApplied || absentee.Email == "" {
				continue
			}

			if existing, ok := unapplied[absentee.Email]; ok {
				existing.dates = append(existing.dates, report.Date)
			} else {
				unapplied[absentee.Email] = &absenteeInfo{
					name:  absentee.EmpName,
					dates: []string{report.Date},
				}
			}
		}
	}
	return unapplied
}

func buildEmailContent(empName string, dates []string) string {
	dateList := ""
	for _, d := range dates {
		dateList += "<li>" + formatDate(d) + "</li>"
	}
	dateList = "<ul>" + dateList + "</ul>"

	content := emailTemplate
	content = strings.ReplaceAll(content, "{{.EmpName}}", empName)
	content = strings.ReplaceAll(content, "{{.Dates}}", dateList)
	return content
}

func formatDate(date string) string {
	// Parse the input string
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date // return the original string if parsing fails
	}
	return t.Format("02-Jan-2006")
}

func sendMail(accessToken, senderEmail, recipientEmail string, ccEmails []string, htmlContent string) error {
	var reqBody EmailRequest
	reqBody.Message.Subject = "Leave Application Reminder"
	reqBody.Message.Body.ContentType = "HTML"
	reqBody.Message.Body.Content = htmlContent
	reqBody.SaveToSentItems = true

	recipient := Recipient{}
	recipient.EmailAddress.Address = recipientEmail
	reqBody.Message.ToRecipients = []Recipient{recipient}

	for _, cc := range ccEmails {
		r := Recipient{}
		r.EmailAddress.Address = cc
		reqBody.Message.CcRecipients = append(reqBody.Message.CcRecipients, r)
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal email request: %w", err)
	}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", senderEmail)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
