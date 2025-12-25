package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// GetFileTypeFromUrl 获取文件类型，返回 mime type， 例如 image/jpeg, image/png, image/gif, image/bmp, image/tiff, application/pdf
// 如果获取失败，返回 application/octet-stream
func GetFileTypeFromUrl(c *gin.Context, url string, reason ...string) (string, error) {
	response, err := DoDownloadRequest(url, []string{"get_mime_type", strings.Join(reason, ", ")}...)
	if err != nil {
		common.SysLog(fmt.Sprintf("fail to get file type from url: %s, error: %s", url, err.Error()))
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		logger.LogError(c, fmt.Sprintf("failed to download file from %s, status code: %d", url, response.StatusCode))
		return "", fmt.Errorf("failed to download file, status code: %d", response.StatusCode)
	}

	if headerType := strings.TrimSpace(response.Header.Get("Content-Type")); headerType != "" {
		if i := strings.Index(headerType, ";"); i != -1 {
			headerType = headerType[:i]
		}
		if headerType != "application/octet-stream" {
			return headerType, nil
		}
	}

	if cd := response.Header.Get("Content-Disposition"); cd != "" {
		parts := strings.Split(cd, ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(strings.ToLower(part), "filename=") {
				name := strings.TrimSpace(strings.TrimPrefix(part, "filename="))
				if len(name) > 2 && name[0] == '"' && name[len(name)-1] == '"' {
					name = name[1 : len(name)-1]
				}
				if dot := strings.LastIndex(name, "."); dot != -1 && dot+1 < len(name) {
					ext := strings.ToLower(name[dot+1:])
					if ext != "" {
						mt := GetMimeTypeByExtension(ext)
						if mt != "application/octet-stream" {
							return mt, nil
						}
					}
				}
				break
			}
		}
	}

	cleanedURL := url
	if q := strings.Index(cleanedURL, "?"); q != -1 {
		cleanedURL = cleanedURL[:q]
	}
	if slash := strings.LastIndex(cleanedURL, "/"); slash != -1 && slash+1 < len(cleanedURL) {
		last := cleanedURL[slash+1:]
		if dot := strings.LastIndex(last, "."); dot != -1 && dot+1 < len(last) {
			ext := strings.ToLower(last[dot+1:])
			if ext != "" {
				mt := GetMimeTypeByExtension(ext)
				if mt != "application/octet-stream" {
					return mt, nil
				}
			}
		}
	}

	var readData []byte
	limits := []int{512, 8 * 1024, 24 * 1024, 64 * 1024}
	for _, limit := range limits {
		logger.LogDebug(c, fmt.Sprintf("Trying to read %d bytes to determine file type", limit))
		if len(readData) < limit {
			need := limit - len(readData)
			tmp := make([]byte, need)
			n, _ := io.ReadFull(response.Body, tmp)
			if n > 0 {
				readData = append(readData, tmp[:n]...)
			}
		}

		if len(readData) == 0 {
			continue
		}

		sniffed := http.DetectContentType(readData)
		if sniffed != "" && sniffed != "application/octet-stream" {
			return sniffed, nil
		}

		if _, format, err := image.DecodeConfig(bytes.NewReader(readData)); err == nil {
			switch strings.ToLower(format) {
			case "jpeg", "jpg":
				return "image/jpeg", nil
			case "png":
				return "image/png", nil
			case "gif":
				return "image/gif", nil
			case "bmp":
				return "image/bmp", nil
			case "tiff":
				return "image/tiff", nil
			default:
				if format != "" {
					return "image/" + strings.ToLower(format), nil
				}
			}
		}
	}

	// Fallback
	return "application/octet-stream", nil
}

func GetFileBase64FromUrl(c *gin.Context, url string, reason ...string) (*types.LocalFileData, error) {
	contextKey := fmt.Sprintf("file_download_%s", common.GenerateHMAC(url))

	// Check if the file has already been downloaded in this request
	if cachedData, exists := c.Get(contextKey); exists {
		if common.DebugEnabled {
			logger.LogDebug(c, fmt.Sprintf("Using cached file data for URL: %s", url))
		}
		return cachedData.(*types.LocalFileData), nil
	}

	var maxFileSize = constant.MaxFileDownloadMB * 1024 * 1024

	resp, err := DoDownloadRequest(url, reason...)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Always use LimitReader to prevent oversized downloads
	fileBytes, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxFileSize+1)))
	if err != nil {
		return nil, err
	}
	// Check actual size after reading
	if len(fileBytes) > maxFileSize {
		return nil, fmt.Errorf("file size exceeds maximum allowed size: %dMB", constant.MaxFileDownloadMB)
	}

	// Convert to base64
	base64Data := base64.StdEncoding.EncodeToString(fileBytes)

	mimeType := resp.Header.Get("Content-Type")
	if len(strings.Split(mimeType, ";")) > 1 {
		// If Content-Type has parameters, take the first part
		mimeType = strings.Split(mimeType, ";")[0]
	}
	if mimeType == "application/octet-stream" {
		logger.LogDebug(c, fmt.Sprintf("MIME type is application/octet-stream for URL: %s", url))
		// try to guess the MIME type from the url last segment
		urlParts := strings.Split(url, "/")
		if len(urlParts) > 0 {
			lastSegment := urlParts[len(urlParts)-1]
			if strings.Contains(lastSegment, ".") {
				// Extract the file extension
				filename := strings.Split(lastSegment, ".")
				if len(filename) > 1 {
					ext := strings.ToLower(filename[len(filename)-1])
					// Guess MIME type based on file extension
					mimeType = GetMimeTypeByExtension(ext)
				}
			}
		} else {
			// try to guess the MIME type from the file extension
			fileName := resp.Header.Get("Content-Disposition")
			if fileName != "" {
				// Extract the filename from the Content-Disposition header
				parts := strings.Split(fileName, ";")
				for _, part := range parts {
					if strings.HasPrefix(strings.TrimSpace(part), "filename=") {
						fileName = strings.TrimSpace(strings.TrimPrefix(part, "filename="))
						// Remove quotes if present
						if len(fileName) > 2 && fileName[0] == '"' && fileName[len(fileName)-1] == '"' {
							fileName = fileName[1 : len(fileName)-1]
						}
						// Guess MIME type based on file extension
						if ext := strings.ToLower(strings.TrimPrefix(fileName, ".")); ext != "" {
							mimeType = GetMimeTypeByExtension(ext)
						}
						break
					}
				}
			}
		}
	}
	data := &types.LocalFileData{
		Base64Data: base64Data,
		MimeType:   mimeType,
		Size:       int64(len(fileBytes)),
	}
	// Store the file data in the context to avoid re-downloading
	c.Set(contextKey, data)

	return data, nil
}

func GetMimeTypeByExtension(ext string) string {
	// Convert to lowercase for case-insensitive comparison
	ext = strings.ToLower(ext)
	switch ext {
	// Text files
	case "txt", "md", "markdown", "csv", "json", "xml", "html", "htm":
		return "text/plain"

	// Image files
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"

	// Audio files
	case "mp3":
		return "audio/mp3"
	case "wav":
		return "audio/wav"
	case "mpeg":
		return "audio/mpeg"

	// Video files
	case "mp4":
		return "video/mp4"
	case "wmv":
		return "video/wmv"
	case "flv":
		return "video/flv"
	case "mov":
		return "video/mov"
	case "mpg":
		return "video/mpg"
	case "avi":
		return "video/avi"
	case "mpegps":
		return "video/mpegps"

	// Document files
	case "pdf":
		return "application/pdf"

	default:
		return "application/octet-stream" // Default for unknown types
	}
}

// URL转换服务管理
var (
	converterURLs      []string
	converterURLsOnce  sync.Once
	converterIndex     int
	converterMutex     sync.Mutex
	converterHTTPClient *http.Client
	converterClientOnce sync.Once
)

// getConverterHTTPClient 获取专用于URL转base64的HTTP客户端（超时2500秒）
func getConverterHTTPClient() *http.Client {
	converterClientOnce.Do(func() {
		converterHTTPClient = &http.Client{
			Timeout: 2500 * time.Second, // 2500秒超时
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableKeepAlives:   false,
			},
		}
		common.SysLog("Initialized URL converter HTTP client with 2500s timeout")
	})
	return converterHTTPClient
}

// getConverterURLs 从环境变量读取转换服务URL列表
func getConverterURLs() []string {
	converterURLsOnce.Do(func() {
		// 从环境变量读取 URL_CONVERTER_BASE_URLS
		urlsEnv := os.Getenv("URL_CONVERTER_BASE_URLS")
		if urlsEnv == "" {
			common.SysLog("Warning: URL_CONVERTER_BASE_URLS environment variable not set, URL to base64 conversion will be disabled")
			converterURLs = []string{}
			return
		}

		// 按逗号分割URL
		urls := strings.Split(urlsEnv, ",")
		validURLs := make([]string, 0, len(urls))

		for _, url := range urls {
			url = strings.TrimSpace(url)
			if url != "" {
				validURLs = append(validURLs, url)
			}
		}

		if len(validURLs) == 0 {
			common.SysLog("Warning: No valid URLs found in URL_CONVERTER_BASE_URLS")
		} else {
			common.SysLog(fmt.Sprintf("Loaded %d URL converter service(s): %v", len(validURLs), validURLs))
		}

		converterURLs = validURLs
	})
	return converterURLs
}

// selectConverterURL 使用轮询负载均衡选择一个转换服务URL
func selectConverterURL() (string, error) {
	urls := getConverterURLs()

	if len(urls) == 0 {
		return "", fmt.Errorf("no URL converter services configured, please set URL_CONVERTER_BASE_URLS environment variable")
	}

	converterMutex.Lock()
	defer converterMutex.Unlock()

	// 轮询选择
	selectedURL := urls[converterIndex%len(urls)]
	converterIndex++

	return selectedURL, nil
}

// GetBase64FromUrlConverter 通过URL转base64转换接口获取base64数据
// 自动从环境变量 URL_CONVERTER_BASE_URLS 读取转换服务列表并负载均衡
// 使用2500秒超时的专用HTTP客户端
func GetBase64FromUrlConverter(c *gin.Context, videoUrl string) (string, error) {
	// 选择一个转换服务URL
	converterBaseUrl, err := selectConverterURL()
	if err != nil {
		return "", err
	}

	// 构建转换接口URL
	converterUrl := fmt.Sprintf("%s/?url=%s", converterBaseUrl, videoUrl)

	logger.LogDebug(c, fmt.Sprintf("Converting URL to base64 via: %s (selected from %d services, timeout: 2500s)", converterBaseUrl, len(getConverterURLs())))

	// 使用专用的HTTP客户端发送请求（2500秒超时）
	client := getConverterHTTPClient()
	resp, err := client.Get(converterUrl)
	if err != nil {
		return "", fmt.Errorf("failed to request URL converter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("URL converter returned status code: %d", resp.StatusCode)
	}

	// 读取响应(应该已经是base64格式)
	base64Data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response from URL converter: %w", err)
	}

	// 清理base64数据：移除所有空白字符（空格、换行符、制表符等）
	// 这很重要，因为某些转换服务可能会在base64中添加换行符
	cleanedBase64 := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1 // 移除这些字符
		}
		return r
	}, string(base64Data))

	return cleanedBase64, nil
}
