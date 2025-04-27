package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/emersion/go-message/mail"
)

type zeit_config struct {
	Zeit_User      string `toml:"zeit_user"`
	Zeit_Pwd       string `toml:"zeit_pwd"`
	Captcha_APIKey string `toml:"captcha_apikey"`
}

const ZEIT_SENDER_ADDRESS = "noreply@digitalabo.mailing.zeit.de"
const CAPTCHA_URL_KEY = "FCMHGOVVTEVINKPD"

func processZeitDownloadNotification(p *mail.Part, config zeit_config) bool {

	log.Println("Found email sent by Zeit")

	// TODO: find a more robust way to get to link
	body, err := io.ReadAll(p.Body)
	if err != nil {
		log.Println(err)
		return false
	}

	mailContent := string(body)
	needleStart := strings.Index(mailContent, "automatisch heruntergeladen")
	if needleStart == -1 {
		log.Println("Could not find link start search marker")
		return false
	}
	mailContent = mailContent[needleStart:]
	indexStart := strings.Index(mailContent, "https://t.mailing.zeit.de/lnk/")
	if indexStart == -1 {
		log.Println("Could not find link marker")
		return false
	}
	mailContent = mailContent[indexStart:]
	indexEnd := strings.Index(mailContent, "\"")
	downloadUrl := mailContent[0:indexEnd]

	log.Println("found download link, starting download:")
	log.Println(downloadUrl)
	return downloadZeitEpub(defaultLibraryPath+"zeit_"+time.Now().Format("02-01-2006")+".epub", downloadUrl, config)
}

func downloadZeitEpub(filepath string, address string, config zeit_config) bool {

	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Println("Error:", err)
		return false
	}

	client := &http.Client{
		Jar: jar,
	}

	log.Println("Getting login page")

	loginpageUrl := "https://meine.zeit.de/anmelden"
	loginpage_resp, err := client.Get(loginpageUrl)
	if err != nil {
		log.Println("Error:", err)
		return false
	}

	finalURLAfterRedirects := loginpage_resp.Request.URL.String()
	log.Println("Final URL:", finalURLAfterRedirects)
	captcha_solution := solveCaptcha(config.Captcha_APIKey, finalURLAfterRedirects)

	login_url, err := extractLoginUrl(loginpage_resp.Body)
	if err != nil {
		log.Println("Error parsing form action url: ", err)
		return false
	}
	log.Println("Login URL:", login_url)

	values := url.Values{}
	values.Set("frc-captcha-response", captcha_solution)
	values.Set("password", config.Zeit_Pwd)
	values.Set("username", config.Zeit_User)

	loginRequest, err := http.NewRequest("POST", login_url, strings.NewReader(values.Encode()))
	if err != nil {
		log.Println("Error:", err)
		return false
	}

	loginRequest.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	log.Println("Logging in")

	resp, err := client.Do(loginRequest)
	if err != nil {
		log.Println("Error:", err)
		return false
	}
	if resp.StatusCode != http.StatusOK {
		log.Println("Could not login: " + resp.Status)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		bodyString := string(bodyBytes)
		log.Println(bodyString)
		return false
	}

	log.Println("Retrieving epub")

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		log.Println("Could not create file", err)
		return false
	}
	defer out.Close()

	// Get the data
	resp, err = client.Get(address)
	if err != nil {
		log.Println("Could not retrieve zeit epub file", err)
		return false
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		log.Println("bad status: " + resp.Status)
		return false
	}

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Println("Could not write to file")
		return false
	}

	return true
}

func solveCaptcha(apiKey string, url string) string {
	client := NewCaptchaClient(apiKey)
	taskID, err := client.CreateTask(url, CAPTCHA_URL_KEY)
	if err != nil {
		log.Println("Error creating task: ", err)
		return ""
	}

	solutionToken, err := client.GetTaskResult(taskID)
	if err != nil {
		log.Println("Error getting task result: ", err)
	}

	log.Println("Solution Token: ", solutionToken)
	return solutionToken
}

func extractLoginUrl(body io.Reader) (string, error) {
	doc, err := html.Parse(body)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	var actionURL string
	var findForm func(*html.Node)
	findForm = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == "kc-form-login" {
					for _, a := range n.Attr {
						if a.Key == "action" {
							actionURL = a.Val
							return
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findForm(c)
		}
	}

	findForm(doc)

	if actionURL == "" {
		return "", fmt.Errorf("login form not found")
	}

	// html entity decoding (e.g., &amp;)
	actionURL = html.UnescapeString(actionURL)
	return actionURL, nil
}
