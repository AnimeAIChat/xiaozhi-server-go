package utils

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
)

var musicNames []string

// MusicMatch 表示音乐文件匹配结果
type MusicMatch struct {
	FilePath   string
	FileName   string
	Similarity float64
}

func checkMusicDirectory(musicDir string) bool {
	// 检查音乐目录是否存在
	if _, err := os.Stat(musicDir); os.IsNotExist(err) {
		return false
	}
	return true
}

// 获取所有歌曲名字
func GetAllMusicNames(musicDir string) ([]string, error) {
	if len(musicNames) > 0 {
		return musicNames, nil
	}

	if !checkMusicDirectory(musicDir) {
		return nil, os.ErrNotExist
	}

	files, err := os.ReadDir(musicDir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() || strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		musicNames = append(musicNames, file.Name())
	}

	return musicNames, nil
}

func IsMusicFile(filePath string) bool {
	if filePath == "" {
		return false
	}
	if strings.Contains(filePath, "/music/") || strings.Contains(filePath, "\\music\\") {
		return true
	}
	return false
}

func getRandomMusicFile(musicDir string) (string, string, error) {
	files, err := GetAllMusicNames(musicDir)
	if err != nil {
		return "", "", err
	}
	if len(files) == 0 {
		return "", "", fmt.Errorf("no music files found in directory '%s'", musicDir)
	}
	// 随机选择一个文件
	randomIndex := rand.Intn(len(files))
	fileName := files[randomIndex]
	// 去掉扩展名
	if dotIndex := strings.LastIndex(fileName, "."); dotIndex != -1 {
		fileName = fileName[:dotIndex]
	}
	return fmt.Sprintf("%s/%s", musicDir, files[randomIndex]), fileName, nil
}

func GetFileNameFromPath(filePath string) string {
	if filePath == "" {
		return ""
	}
	// 获取文件名部分
	fileName := filePath[strings.LastIndex(filePath, "/")+1:]
	fileName = fileName[strings.LastIndex(fileName, "\\")+1:] // 处理Windows路径
	// 去掉扩展名
	if dotIndex := strings.LastIndex(fileName, "."); dotIndex != -1 {
		fileName = fileName[:dotIndex]
	}
	return fileName
}

// 根据音乐文件名获取音乐文件路径（模糊匹配）
func GetMusicFilePathFuzzy(songName string) (string, string, error) {
	musicDir := "./music"

	if songName == "random" || songName == "随机" {
		// 如果是随机请求，直接返回一个随机音乐文件
		return getRandomMusicFile(musicDir)
	}

	if !checkMusicDirectory(musicDir) {
		return "", "", os.ErrNotExist
	}

	// 首先尝试精确匹配
	filePath := fmt.Sprintf("%s/%s.mp3", musicDir, songName)
	// 如果存在，则直接返回
	if _, err := os.Stat(filePath); err == nil {
		return filePath, songName, nil
	}

	// 获取所有音乐文件
	files, err := GetAllMusicNames(musicDir)
	if err != nil {
		return "", "", err
	}

	var bestMatch MusicMatch
	bestMatch.Similarity = 0.0

	normalizedInput := normalizeString(songName)

	// 对每个文件计算相似度
	for _, file := range files {
		fileName := file
		fileNameWithoutExt := strings.TrimSuffix(fileName, ".mp3")
		normalizedFileName := normalizeString(fileNameWithoutExt)

		similarity := calculateSimilarity(normalizedInput, normalizedFileName)

		if similarity > bestMatch.Similarity {
			bestMatch.FilePath = fmt.Sprintf("%s/%s", musicDir, fileName)
			bestMatch.FileName = fileName
			bestMatch.Similarity = similarity
		}
	}

	// 如果最佳匹配的相似度大于等于0.5，返回结果
	if bestMatch.Similarity >= 0.5 {
		fileName := GetFileNameFromPath(bestMatch.FilePath)
		return bestMatch.FilePath, fileName, nil
	}

	return "", "", fmt.Errorf(
		"no music file found matching '%s' (best similarity: %.2f)",
		songName,
		bestMatch.Similarity,
	)
}

// normalizeString 标准化字符串，去除特殊字符和空格，转换为小写
func normalizeString(s string) string {
	var result strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r >= 0x4e00 && r <= 0x9fff {
			result.WriteRune(r)
		}
	}
	return strings.ToLower(result.String())
}

// calculateSimilarity 计算两个字符串的相似度（0-1之间）
func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// 包含匹配检查
	containsSimilarity := 0.0
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		shorter := len(s1)
		longer := len(s2)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		containsSimilarity = float64(shorter) / float64(longer)
	}

	// 编辑距离相似度
	editDist := editDistance(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}
	editSimilarity := 1.0 - float64(editDist)/float64(maxLen)

	// 最长公共子序列相似度
	lcsLen := longestCommonSubsequence(s1, s2)
	lcsSimilarity := float64(lcsLen*2) / float64(len(s1)+len(s2))

	// 综合相似度（权重分配）
	finalSimilarity := containsSimilarity*0.3 + editSimilarity*0.4 + lcsSimilarity*0.3

	if finalSimilarity > 1.0 {
		finalSimilarity = 1.0
	}

	return finalSimilarity
}

// editDistance 计算编辑距离
func editDistance(s1, s2 string) int {
	m, n := len(s1), len(s2)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 0; i <= m; i++ {
		dp[i][0] = i
	}
	for j := 0; j <= n; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if s1[i-1] == s2[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = min(dp[i-1][j], dp[i][j-1], dp[i-1][j-1]) + 1
			}
		}
	}

	return dp[m][n]
}

// longestCommonSubsequence 计算最长公共子序列长度
func longestCommonSubsequence(s1, s2 string) int {
	m, n := len(s1), len(s2)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if s1[i-1] == s2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	return dp[m][n]
}

// min 返回三个整数中的最小值
func min(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// max 返回两个整数中的最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
