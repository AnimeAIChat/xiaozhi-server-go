package face

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// FaceData 人脸数据结构
type FaceData struct {
	UserID      string    `json:"user_id"`
	UserName    string    `json:"user_name"`
	Feature     string    `json:"feature"`
	Base64Image string    `json:"base64_image,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	IsActive    bool      `json:"is_active"`
}

// FaceDatabase 人脸数据库
type FaceDatabase struct {
	mu    sync.RWMutex
	faces map[string]*FaceData
	file  string
}

// NewFaceDatabase 创建新的人脸数据库
func NewFaceDatabase() *FaceDatabase {
	db := &FaceDatabase{
		faces: make(map[string]*FaceData),
		file:  "data/face_database.json",
	}
	db.load()
	return db
}

// AddFaceWithImage 添加人脸数据（包含图像）
func (db *FaceDatabase) AddFaceWithImage(userID, userName, feature, base64Image string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	faceData := &FaceData{
		UserID:      userID,
		UserName:    userName,
		Feature:     feature,
		Base64Image: base64Image,
		CreatedAt:   time.Now(),
		IsActive:    true,
	}

	db.faces[userID] = faceData
	return db.save()
}

// DeleteFace 删除人脸数据
func (db *FaceDatabase) DeleteFace(userID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.faces[userID]; !exists {
		return fmt.Errorf("用户 %s 不存在", userID)
	}

	delete(db.faces, userID)
	return db.save()
}

// GetAllFaces 获取所有人脸数据
func (db *FaceDatabase) GetAllFaces() []*FaceData {
	db.mu.RLock()
	defer db.mu.RUnlock()

	faces := make([]*FaceData, 0, len(db.faces))
	for _, face := range db.faces {
		faces = append(faces, face)
	}
	return faces
}

// ToActiveJSON 返回活跃人脸数据的JSON
func (db *FaceDatabase) ToActiveJSON() ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	activeFaces := make([]*FaceData, 0)
	for _, face := range db.faces {
		if face.IsActive {
			activeFaces = append(activeFaces, face)
		}
	}

	response := map[string]interface{}{
		"type":        "face_db_sync",
		"success":     true,
		"faces":       activeFaces,
		"total_count": len(activeFaces),
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	return json.Marshal(response)
}

// load 从文件加载数据
func (db *FaceDatabase) load() error {
	if _, err := os.Stat(db.file); os.IsNotExist(err) {
		return nil // 文件不存在，使用空数据库
	}

	data, err := os.ReadFile(db.file)
	if err != nil {
		return err
	}

	var fileData struct {
		Faces []*FaceData `json:"faces"`
	}

	if err := json.Unmarshal(data, &fileData); err != nil {
		return err
	}

	for _, face := range fileData.Faces {
		db.faces[face.UserID] = face
	}

	return nil
}

// save 保存数据到文件
func (db *FaceDatabase) save() error {
	// 确保目录存在
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}

	faces := make([]*FaceData, 0, len(db.faces))
	for _, face := range db.faces {
		faces = append(faces, face)
	}

	fileData := struct {
		Faces []*FaceData `json:"faces"`
	}{
		Faces: faces,
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(db.file, data, 0644)
}
