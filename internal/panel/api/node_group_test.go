package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/panel/db"
)

func TestNodeGroupHandlerCreateListDelete(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	handler := &NodeGroupHandler{DB: database}

	createBody, _ := json.Marshal(map[string]any{
		"name":        "entry-hk",
		"description": "香港入口组",
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/node-groups", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRec)
	createCtx.Request = createReq
	createCtx.Set("username", "tester")
	handler.CreateNodeGroup(createCtx)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected create status 200, got %d: %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/node-groups", nil)
	listRec := httptest.NewRecorder()
	listCtx, _ := gin.CreateTestContext(listRec)
	listCtx.Request = listReq
	handler.ListNodeGroups(listCtx)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp struct {
		NodeGroups []map[string]any `json:"node_groups"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.NodeGroups) != 1 {
		t.Fatalf("expected 1 node group, got %d", len(listResp.NodeGroups))
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/node-groups/1", nil)
	deleteRec := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRec)
	deleteCtx.Params = gin.Params{{Key: "id", Value: "1"}}
	deleteCtx.Request = deleteReq
	deleteCtx.Set("username", "tester")
	handler.DeleteNodeGroup(deleteCtx)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete status 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
}

func newTestAPIContextDatabase(t *testing.T) *db.Database {
	t.Helper()

	database, err := db.New(t.TempDir() + "/flowgate-api.db")
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo to work") {
			t.Skip("sqlite cgo runtime unavailable in this test environment")
		}
		t.Fatalf("new test database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database
}
