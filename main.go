package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	LoadEnv()
	apiKey := GetAPIKey()
	assistantID := "asst_Ht8x8n4tcGzx5p6HPb29FUvC" // Replace with your Assistant ID
	dirPath := "c:\\aistrat\\bank"                 // Updated directory path

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fmt.Println("Processing file:", path)

			fileID, err := uploadFile(path, apiKey)
			if err != nil {
				fmt.Println("File upload failed for", path, ":", err)
				return nil
			}

			fmt.Println("Uploaded File ID:", fileID)

			result, err := runAssistant(fileID, assistantID, apiKey)
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
