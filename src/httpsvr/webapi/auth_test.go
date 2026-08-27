package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthMiddlewareMapsJWTToDefaultUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:single-user-auth-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{ID: database.AdminUserID, Username: "admin", Role: "admin", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	token, err := GenerateJWT(99, "legacy-user")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/protected", func(c *gin.Context) {
		c.String(http.StatusOK, "%d", c.GetUint("user_id"))
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "1" {
		t.Fatalf("authentication user = %q (status %d), want default user 1", recorder.Body.String(), recorder.Code)
	}
}
