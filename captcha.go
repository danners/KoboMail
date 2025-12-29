package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"
)

type CaptchaClient struct {
	APIKey string
	Client *http.Client
}

type createTaskResponse struct {
	ErrorId int   `json:"errorId"`
	TaskId  int64 `json:"taskId"`
}

type taskResultResponse struct {
	ErrorId  int    `json:"errorId"`
	Status   string `json:"status"`
	Solution struct {
		Token string `json:"token"`
	} `json:"solution"`
}

func NewCaptchaClient(apiKey string, skipSSL bool) *CaptchaClient {
	return &CaptchaClient{
		APIKey: apiKey,
		Client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: skipSSL},
			},
		},
	}
}

func (c *CaptchaClient) CreateTask(websiteURL, websiteKey string) (int64, error) {
	requestBody, _ := json.Marshal(map[string]interface{}{
		"clientKey": c.APIKey,
		"task": map[string]string{
			"type":       "FriendlyCaptchaTaskProxyless",
			"websiteURL": websiteURL,
			"websiteKey": websiteKey,
			"version":    "v2",
		},
	})

	resp, err := c.Client.Post("https://api.2captcha.com/createTask", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var response createTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, err
	}

	if response.ErrorId != 0 {
		return 0, errors.New("failed to create task")
	}

	return response.TaskId, nil
}

func (c *CaptchaClient) GetTaskResult(taskId int64) (string, error) {
	requestBody, _ := json.Marshal(map[string]interface{}{
		"clientKey": c.APIKey,
		"taskId":    taskId,
	})

	for {
		resp, err := c.Client.Post("https://api.2captcha.com/getTaskResult", "application/json", bytes.NewBuffer(requestBody))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		var response taskResultResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return "", err
		}

		if response.ErrorId != 0 {
			return "", errors.New("failed to get task result")
		}

		if response.Status == "ready" {
			return response.Solution.Token, nil
		}

		log.Println("captcha task not ready, waiting...")

		time.Sleep(5 * time.Second)
	}
}
