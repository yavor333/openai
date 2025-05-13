package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

func uploadFile(filePath string, apiKey string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(file.Name()))
	if err != nil {
		return "", err
	}
	_, err = io.Copy(part, file)
	writer.Close()

	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/files", body)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return result["id"].(string), nil
}

func runAssistant(fileID string, assistantID string, apiKey string) (string, error) {
	threadReq := map[string]interface{}{}
	threadBody, _ := json.Marshal(threadReq)

	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/threads", bytes.NewBuffer(threadBody))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	var thread map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&thread)
	threadID := thread["id"].(string)

	messageReq := map[string]interface{}{
		"file_ids": []string{fileID},
		"content":  "Please process the file as per system instructions.",
		"role":     "user",
	}
	messageBody, _ := json.Marshal(messageReq)
	messageURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/messages", threadID)
	req2, _ := http.NewRequest("POST", messageURL, bytes.NewBuffer(messageBody))
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	req2.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req2)

	runReq := map[string]interface{}{
		"assistant_id": assistantID,
	}
	runBody, _ := json.Marshal(runReq)
	runURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs", threadID)
	req3, _ := http.NewRequest("POST", runURL, bytes.NewBuffer(runBody))
	req3.Header.Set("Authorization", "Bearer "+apiKey)
	req3.Header.Set("Content-Type", "application/json")
	resp3, _ := http.DefaultClient.Do(req3)
	defer resp3.Body.Close()

	var run map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&run)
	runID := run["id"].(string)

	for {
		statusURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs/%s", threadID, runID)
		reqStatus, _ := http.NewRequest("GET", statusURL, nil)
		reqStatus.Header.Set("Authorization", "Bearer "+apiKey)
		respStatus, _ := http.DefaultClient.Do(reqStatus)
		defer respStatus.Body.Close()

		var statusResp map[string]interface{}
		json.NewDecoder(respStatus.Body).Decode(&statusResp)
		if statusResp["status"] == "completed" {
			break
		}
	}

	msgURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/messages", threadID)
	reqMsg, _ := http.NewRequest("GET", msgURL, nil)
	reqMsg.Header.Set("Authorization", "Bearer "+apiKey)
	respMsg, _ := http.DefaultClient.Do(reqMsg)
	defer respMsg.Body.Close()

	var msgs map[string]interface{}
	json.NewDecoder(respMsg.Body).Decode(&msgs)

	messages := msgs["data"].([]interface{})
	latest := messages[0].(map[string]interface{})
	content := latest["content"].([]interface{})[0].(map[string]interface{})
	return content["text"].(map[string]interface{})["value"].(string), nil
}
