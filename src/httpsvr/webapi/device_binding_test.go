package webapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeviceBindingOwnershipAndDisable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:device-binding-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.Device{}); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	agent := &models.Agent{Name: "我的智能体", UserID: 1}
	if err := db.Create(agent).Error; err != nil {
		t.Fatal(err)
	}
	otherAgent := &models.Agent{Name: "其他用户智能体", UserID: 2}
	if err := db.Create(otherAgent).Error; err != nil {
		t.Fatal(err)
	}
	device := &models.Device{Name: "测试设备", DeviceID: "device-001", ClientID: "client-001"}
	if err := db.Create(device).Error; err != nil {
		t.Fatal(err)
	}

	service := &DefaultUserService{}
	bind := func(userID, agentID uint) *httptest.ResponseRecorder {
		body, _ := json.Marshal(DeviceBindRequest{AgentID: agentID, DeviceID: device.DeviceID})
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/user/device/bind", bytes.NewReader(body))
		context.Request.Header.Set("Content-Type", "application/json")
		context.Set("user_id", userID)
		service.handleDeviceBind(context)
		return recorder
	}

	if response := bind(1, agent.ID); response.Code != http.StatusOK {
		t.Fatalf("first bind status = %d, body = %s", response.Code, response.Body.String())
	}
	if err := db.First(device, "device_id = ?", device.DeviceID).Error; err != nil {
		t.Fatal(err)
	}
	if device.UserID == nil || *device.UserID != 1 || device.AgentID == nil || *device.AgentID != agent.ID {
		t.Fatalf("device ownership was not saved: %#v", device)
	}
	if response := bind(2, otherAgent.ID); response.Code != http.StatusForbidden {
		t.Fatalf("cross-user bind status = %d, want %d", response.Code, http.StatusForbidden)
	}

	disabled := true
	body, _ := json.Marshal(DeviceUpdateRequest{Disabled: &disabled})
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/user/device/device-001", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Params = gin.Params{{Key: "id", Value: device.DeviceID}}
	context.Set("user_id", uint(1))
	service.handleDeviceUpdate(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(device, "device_id = ?", device.DeviceID).Error; err != nil {
		t.Fatal(err)
	}
	if device.Mode != "ban" {
		t.Fatalf("device mode = %q, want ban", device.Mode)
	}
}
