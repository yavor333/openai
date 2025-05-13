package main

import (
	"fmt"
	"new/openai"
	"new/util"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	util.LoadEnv()
	apiKey := util.GetAPIKey()
	assistantID := "asst_Ht8x8n4tcGzx5p6HPb29FUvC"
	dirPath := "c:\\aistrat\\openai"
	outputDir := "c:\\aistrat\\bank_output"
	// Ensure output directory exists
	os.MkdirAll(outputDir, 0755)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fmt.Println("Processing file:", path)

			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".xls" {
				fmt.Println("Skipping unsupported file type:", path)
				return nil
			}

			var fileType string
			switch ext {
			case ".csv":
				fileType = "CSV"
				fmt.Println("Detected CSV file:", path)
			case ".html", ".htm":
				fileType = "HTML"
			default:
				fileType = "unknown"
			}

			fmt.Println("Attempting to upload file:", path)
			fileID, err := openai.UploadFile(path, apiKey)
			if err != nil {
				fmt.Println("File upload failed for", path, ":", err)
				return nil
			}

			fmt.Println("Uploaded File ID:", fileID)

			result, err := openai.RunAssistant(fileID, assistantID, apiKey, path, outputDir, fileType)
			if err != nil {
				fmt.Println("Assistant call failed for", path, ":", err)
				return nil
			}

			fmt.Println("Assistant Response for", path, ":")
			fmt.Println(result)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error walking the directory:", err)
		os.Exit(1)
	}
}
