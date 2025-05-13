package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// UploadFile uploads a file to OpenAI and returns its file ID
func UploadFile(filePath string, apiKey string) (string, error) {
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
	writer.WriteField("purpose", "assistants")
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
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	// Debug log conditional
	if os.Getenv("DEBUG") == "1" {
		fmt.Printf("Debug: Upload response for %s: %+v\n", filePath, result)
	}

	if resp.StatusCode != 200 {
		bodyBytes, _ := json.MarshalIndent(result, "", "  ")
		return "", fmt.Errorf("upload failed [%d]: %s", resp.StatusCode, string(bodyBytes))
	}

	idRaw, ok := result["id"]
	if !ok {
		return "", fmt.Errorf("upload response missing 'id': %v", result)
	}

	idStr, ok := idRaw.(string)
	if !ok {
		return "", fmt.Errorf("upload response 'id' is not a string: %v", idRaw)
	}

	return idStr, nil
}

// RunAssistant sends a file to an assistant and returns the JSON response
func RunAssistant(fileID string, assistantID string, apiKey string, originalFilePath string, outputDir string, fileType string) (string, error) {
	threadReq := map[string]interface{}{}
	threadBody, _ := json.Marshal(threadReq)

	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/threads", bytes.NewBuffer(threadBody))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "assistants=v2") // Add required header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var thread map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&thread)
	if _, ok := thread["id"]; !ok {
		fmt.Printf("Debug: Thread creation response: %+v\n", thread)
		return "", fmt.Errorf("thread creation response missing 'id': %v", thread)
	}
	threadID := thread["id"].(string)

	// Send the message (without file_ids)
	messageReq := map[string]interface{}{
		"content": fmt.Sprintf(
			`The uploaded file is a %s file containing a financial table. Please read the uploaded file and return its contents as JSON. If the file name starts with BG18, then currency code must be BGN, 
If the file name starts with BG29, then currency code must be EUR
If the file name starts with BG71, then currency code must be USD
otherwise find it in the provided files. 
Return dates in mm/dd/yyyy format.
Decimal separator must be a dot`, fileType),
		"role": "user",
	}
	messageBody, _ := json.Marshal(messageReq)
	messageURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/messages", threadID)
	req2, _ := http.NewRequest("POST", messageURL, bytes.NewBuffer(messageBody))
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("OpenAI-Beta", "assistants=v2") // Keep the required header

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		var body map[string]interface{}
		_ = json.NewDecoder(resp2.Body).Decode(&body)
		return "", fmt.Errorf("message post failed: %v", body)
	}

	runReq := map[string]interface{}{
		"assistant_id": assistantID,
		"tool_resources": map[string]interface{}{ // Correct format for tool_resources
			"code_interpreter": map[string]interface{}{
				"file_ids": []string{fileID}, // Attach your uploaded file here
			},
		},
	}
	runBody, _ := json.Marshal(runReq)
	runURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs", threadID)
	req3, _ := http.NewRequest("POST", runURL, bytes.NewBuffer(runBody))
	req3.Header.Set("Authorization", "Bearer "+apiKey)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("OpenAI-Beta", "assistants=v2") // Add required header

	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		return "", err
	}
	defer resp3.Body.Close()

	var run map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&run)
	if _, ok := run["id"]; !ok {
		return "", fmt.Errorf("run response missing 'id': %v", run)
	}
	runID := run["id"].(string)

	// Poll until assistant finishes, with timeout
	maxRetries := 30 // Retry for 30 * 2s = 60 seconds max
	for retries := 0; retries < maxRetries; retries++ {
		time.Sleep(2 * time.Second)

		statusURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/runs/%s", threadID, runID)
		reqStatus, _ := http.NewRequest("GET", statusURL, nil)
		reqStatus.Header.Set("Authorization", "Bearer "+apiKey)
		reqStatus.Header.Set("OpenAI-Beta", "assistants=v2")

		respStatus, err := http.DefaultClient.Do(reqStatus)
		if err != nil {
			return "", fmt.Errorf("failed to get run status: %w", err)
		}
		defer respStatus.Body.Close()

		var statusResp map[string]interface{}
		if err := json.NewDecoder(respStatus.Body).Decode(&statusResp); err != nil {
			return "", fmt.Errorf("error decoding status response: %w", err)
		}

		status := statusResp["status"]
		fmt.Printf("🔄 Run status: %v\n", status)

		if status == "completed" {
			break
		} else if status == "failed" || status == "cancelled" || status == "expired" {
			fmt.Printf("❌ Run failed details: %+v\n", statusResp)
			return "", fmt.Errorf("run failed with status: %v", status)
		}

		if retries == maxRetries-1 {
			return "", fmt.Errorf("timeout waiting for assistant run to complete")
		}
	}

	// Get the assistant's reply
	msgURL := fmt.Sprintf("https://api.openai.com/v1/threads/%s/messages", threadID)
	reqMsg, _ := http.NewRequest("GET", msgURL, nil)
	reqMsg.Header.Set("Authorization", "Bearer "+apiKey)
	reqMsg.Header.Set("OpenAI-Beta", "assistants=v2") // Add required header

	respMsg, err := http.DefaultClient.Do(reqMsg)
	if err != nil {
		return "", fmt.Errorf("failed to get assistant reply: %w", err)
	}
	defer respMsg.Body.Close()

	if respMsg.StatusCode != 200 {
		var body map[string]interface{}
		_ = json.NewDecoder(respMsg.Body).Decode(&body)
		return "", fmt.Errorf("message retrieval failed: %v", body)
	}

	var msgs map[string]interface{}
	err = json.NewDecoder(respMsg.Body).Decode(&msgs)
	if err != nil {
		return "", fmt.Errorf("error decoding message response: %w", err)
	}

	data, ok := msgs["data"].([]interface{})
	if !ok || len(data) == 0 {
		return "", fmt.Errorf("message response missing 'data': %v", msgs)
	}
	latest := data[0].(map[string]interface{})
	content := latest["content"].([]interface{})[0].(map[string]interface{})

	result := content["text"].(map[string]interface{})["value"].(string)

	// Save to JSON file in outputDir
	outputFile := filepath.Join(outputDir, filepath.Base(originalFilePath)+".json")
	err = os.WriteFile(outputFile, []byte(result), 0644)
	if err != nil {
		fmt.Printf("⚠️ Failed to save output to %s: %v\n", outputFile, err)
	} else {
		fmt.Printf("✅ Assistant output saved to: %s\n", outputFile)
	}

	return result, nil
}
