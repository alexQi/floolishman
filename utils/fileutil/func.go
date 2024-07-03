package fileutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func CreateDir(dirs ...string) (err error) {
	for _, v := range dirs {
		exist, err := PathExists(v)
		if err != nil {
			return err
		}
		if !exist {
			fmt.Printf("create directory\n" + v)
			if err := os.MkdirAll(v, os.ModePerm); err != nil {
				fmt.Printf("create directory\n"+v, err)
				return err
			}
		}
	}
	return err
}

func SaveImage(url, directory string) (string, error) {
	// Get the file name from the URL
	tokens := strings.Split(url, "/")
	filename := tokens[len(tokens)-1]

	// Create the directory if it does not exist
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		err = os.MkdirAll(directory, os.ModePerm)
		if err != nil {
			return "", err
		}
	}

	// Create the file to save the image
	path := filepath.Join(directory, filename)
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Get the image from the URL
	response, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	// Write the image to the file
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return "", err
	}

	return path, nil
}

func DeleteImage(filePath string) error {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("文件 %s 不存在", filePath)
		}
		return err
	}

	// 删除文件
	err := os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("删除文件 %s 失败: %v", filePath, err)
	}
	return nil
}
