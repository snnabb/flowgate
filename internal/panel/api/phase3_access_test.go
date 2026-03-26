package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/flowgate/flowgate/internal/panel/hub"
	"github.com/flowgate/flowgate/internal/panel/model"
)

func TestDisabledUserLoginRejected(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if _, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "disabled-user",
		Role:     "user",
		Enabled:  boolPtr(false),
	}, string(passwordHash)); err != nil {
		t.Fatalf("create disabled user: %v", err)
	}

	handler := &AuthHandler{DB: database, JWTSecret: "test-secret"}
	body, _ := json.Marshal(map[string]any{
		"username": "disabled-user",
		"password": "secret123",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req

	handler.Login(ctx)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled login status 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminUserScopeForNodesAndRules(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	admin, userOne, userTwo := seedAdminAndUsers(t, database)

	nodeOne, err := database.CreateNode("user-one-node", "")
	if err != nil {
		t.Fatalf("create node one: %v", err)
	}
	nodeTwo, err := database.CreateNode("user-two-node", "")
	if err != nil {
		t.Fatalf("create node two: %v", err)
	}

	if err := database.ReplaceUserNodeAccess(userOne.ID, []model.UserNodeAccessInput{
		{NodeID: nodeOne.ID, TrafficQuota: 1024, BandwidthLimit: 1024},
	}); err != nil {
		t.Fatalf("assign node one: %v", err)
	}
	if err := database.ReplaceUserNodeAccess(userTwo.ID, []model.UserNodeAccessInput{
		{NodeID: nodeTwo.ID, TrafficQuota: 1024, BandwidthLimit: 1024},
	}); err != nil {
		t.Fatalf("assign node two: %v", err)
	}

	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     nodeOne.ID,
		Name:       "user-one-rule",
		Protocol:   "tcp",
		ListenPort: 33001,
		TargetAddr: "127.0.0.1",
		TargetPort: 8080,
	}, userOne.ID); err != nil {
		t.Fatalf("create user-one rule: %v", err)
	}
	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     nodeTwo.ID,
		Name:       "user-two-rule",
		Protocol:   "tcp",
		ListenPort: 33002,
		TargetAddr: "127.0.0.1",
		TargetPort: 8081,
	}, userTwo.ID); err != nil {
		t.Fatalf("create user-two rule: %v", err)
	}

	nodeHandler := &NodeHandler{DB: database}
	ruleHandler := &RuleHandler{DB: database}

	userNodesReq := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	userNodesRec := httptest.NewRecorder()
	userNodesCtx, _ := gin.CreateTestContext(userNodesRec)
	userNodesCtx.Request = userNodesReq
	setCurrentUser(userNodesCtx, userOne)
	nodeHandler.ListNodes(userNodesCtx)
	if userNodesRec.Code != http.StatusOK {
		t.Fatalf("list user nodes status = %d: %s", userNodesRec.Code, userNodesRec.Body.String())
	}

	var userNodesResp struct {
		Nodes []model.Node `json:"nodes"`
	}
	if err := json.Unmarshal(userNodesRec.Body.Bytes(), &userNodesResp); err != nil {
		t.Fatalf("decode user nodes response: %v", err)
	}
	if got, want := collectAPIStringsFromNodes(userNodesResp.Nodes), []string{"user-one-node"}; !sameAPIStrings(got, want) {
		t.Fatalf("unexpected user-visible nodes: got %v want %v", got, want)
	}

	userRulesReq := httptest.NewRequest(http.MethodGet, "/api/rules", nil)
	userRulesRec := httptest.NewRecorder()
	userRulesCtx, _ := gin.CreateTestContext(userRulesRec)
	userRulesCtx.Request = userRulesReq
	setCurrentUser(userRulesCtx, userOne)
	ruleHandler.ListRules(userRulesCtx)
	if userRulesRec.Code != http.StatusOK {
		t.Fatalf("list user rules status = %d: %s", userRulesRec.Code, userRulesRec.Body.String())
	}

	var userRulesResp struct {
		Rules []model.Rule `json:"rules"`
	}
	if err := json.Unmarshal(userRulesRec.Body.Bytes(), &userRulesResp); err != nil {
		t.Fatalf("decode user rules response: %v", err)
	}
	if got, want := collectAPIStringsFromRules(userRulesResp.Rules), []string{"user-one-rule"}; !sameAPIStrings(got, want) {
		t.Fatalf("unexpected user-visible rules: got %v want %v", got, want)
	}

	adminRulesReq := httptest.NewRequest(http.MethodGet, "/api/rules", nil)
	adminRulesRec := httptest.NewRecorder()
	adminRulesCtx, _ := gin.CreateTestContext(adminRulesRec)
	adminRulesCtx.Request = adminRulesReq
	setCurrentUser(adminRulesCtx, admin)
	ruleHandler.ListRules(adminRulesCtx)
	if adminRulesRec.Code != http.StatusOK {
		t.Fatalf("list admin rules status = %d: %s", adminRulesRec.Code, adminRulesRec.Body.String())
	}

	var adminRulesResp struct {
		Rules []model.Rule `json:"rules"`
	}
	if err := json.Unmarshal(adminRulesRec.Body.Bytes(), &adminRulesResp); err != nil {
		t.Fatalf("decode admin rules response: %v", err)
	}
	if got, want := collectAPIStringsFromRules(adminRulesResp.Rules), []string{"user-one-rule", "user-two-rule"}; !sameAPIStrings(got, want) {
		t.Fatalf("unexpected admin-visible rules: got %v want %v", got, want)
	}
}

func TestUserCannotAccessManagerRoute(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	_, user, _ := seedAdminAndUsers(t, database)
	nodeHandler := &NodeHandler{DB: database}

	router := gin.New()
	router.POST("/api/nodes",
		injectCurrentUser(user),
		ManagerMiddleware(),
		nodeHandler.CreateNode,
	)

	body, _ := json.Marshal(map[string]any{
		"name": "blocked-node",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/nodes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected user create node to be forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminCanUpdateUserStatusAndAssignments(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	admin, user, _ := seedAdminAndUsers(t, database)
	node, err := database.CreateNode("assignable-node", "")
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	handler := &UserHandler{DB: database}

	updateBody, _ := json.Marshal(model.UpdateUserRequest{
		Enabled: boolPtr(false),
	})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/users/"+strconv.FormatInt(user.ID, 10), bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	updateCtx, _ := gin.CreateTestContext(updateRec)
	updateCtx.Request = updateReq
	updateCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(user.ID, 10)}}
	setCurrentUser(updateCtx, admin)
	handler.UpdateUser(updateCtx)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update user status 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	updated, err := database.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("get updated user: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected user to be disabled")
	}

	replaceBody, _ := json.Marshal(model.ReplaceUserNodeAccessRequest{
		Access: []model.UserNodeAccessInput{
			{NodeID: node.ID, TrafficQuota: 2048, BandwidthLimit: 1024},
		},
	})
	replaceReq := httptest.NewRequest(http.MethodPut, "/api/users/"+strconv.FormatInt(user.ID, 10)+"/access", bytes.NewReader(replaceBody))
	replaceReq.Header.Set("Content-Type", "application/json")
	replaceRec := httptest.NewRecorder()
	replaceCtx, _ := gin.CreateTestContext(replaceRec)
	replaceCtx.Request = replaceReq
	replaceCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(user.ID, 10)}}
	setCurrentUser(replaceCtx, admin)
	handler.ReplaceUserAccess(replaceCtx)

	if replaceRec.Code != http.StatusOK {
		t.Fatalf("expected replace user access 200, got %d: %s", replaceRec.Code, replaceRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/users/"+strconv.FormatInt(user.ID, 10)+"/access", nil)
	getRec := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRec)
	getCtx.Request = getReq
	getCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(user.ID, 10)}}
	setCurrentUser(getCtx, admin)
	handler.GetUserAccess(getCtx)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get user access 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var accessResp struct {
		Access []model.UserNodeAccess `json:"access"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &accessResp); err != nil {
		t.Fatalf("decode user access response: %v", err)
	}
	if len(accessResp.Access) != 1 {
		t.Fatalf("expected 1 access row, got %d", len(accessResp.Access))
	}
	if accessResp.Access[0].NodeID != node.ID {
		t.Fatalf("expected assigned node %d, got %d", node.ID, accessResp.Access[0].NodeID)
	}
}

func TestUserCreateRuleRequiresAssignedNodeAccessAndBandwidthLimit(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	_, user, _ := seedAdminAndUsers(t, database)
	allowedNode, err := database.CreateNode("allowed-node", "")
	if err != nil {
		t.Fatalf("create allowed node: %v", err)
	}
	deniedNode, err := database.CreateNode("denied-node", "")
	if err != nil {
		t.Fatalf("create denied node: %v", err)
	}

	if err := database.ReplaceUserNodeAccess(user.ID, []model.UserNodeAccessInput{
		{NodeID: allowedNode.ID, TrafficQuota: 4096, BandwidthLimit: 1024},
	}); err != nil {
		t.Fatalf("assign allowed node: %v", err)
	}

	ruleHandler := &RuleHandler{DB: database, Hub: hub.New(database)}

	createBody, _ := json.Marshal(map[string]any{
		"node_id":     allowedNode.ID,
		"name":        "default-bandwidth-rule",
		"protocol":    "tcp",
		"listen_port": 35001,
		"target_addr": "127.0.0.1",
		"target_port": 8080,
		"speed_limit": 0,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/rules", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRec)
	createCtx.Request = createReq
	setCurrentUser(createCtx, user)
	ruleHandler.CreateRule(createCtx)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected default-bandwidth create 200, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		Rule model.Rule `json:"rule"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create rule response: %v", err)
	}
	if createResp.Rule.SpeedLimit != 1024 {
		t.Fatalf("expected default speed limit 1024, got %d", createResp.Rule.SpeedLimit)
	}

	deniedBody, _ := json.Marshal(map[string]any{
		"node_id":     deniedNode.ID,
		"name":        "denied-rule",
		"protocol":    "tcp",
		"listen_port": 35002,
		"target_addr": "127.0.0.1",
		"target_port": 8081,
		"speed_limit": 512,
	})
	deniedReq := httptest.NewRequest(http.MethodPost, "/api/rules", bytes.NewReader(deniedBody))
	deniedReq.Header.Set("Content-Type", "application/json")
	deniedRec := httptest.NewRecorder()
	deniedCtx, _ := gin.CreateTestContext(deniedRec)
	deniedCtx.Request = deniedReq
	setCurrentUser(deniedCtx, user)
	ruleHandler.CreateRule(deniedCtx)

	if deniedRec.Code != http.StatusBadRequest && deniedRec.Code != http.StatusNotFound && deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected denied-node create to fail, got %d: %s", deniedRec.Code, deniedRec.Body.String())
	}

	excessBody, _ := json.Marshal(map[string]any{
		"node_id":     allowedNode.ID,
		"name":        "too-fast-rule",
		"protocol":    "tcp",
		"listen_port": 35003,
		"target_addr": "127.0.0.1",
		"target_port": 8082,
		"speed_limit": 2048,
	})
	excessReq := httptest.NewRequest(http.MethodPost, "/api/rules", bytes.NewReader(excessBody))
	excessReq.Header.Set("Content-Type", "application/json")
	excessRec := httptest.NewRecorder()
	excessCtx, _ := gin.CreateTestContext(excessRec)
	excessCtx.Request = excessReq
	setCurrentUser(excessCtx, user)
	ruleHandler.CreateRule(excessCtx)

	if excessRec.Code != http.StatusBadRequest {
		t.Fatalf("expected excess-bandwidth create to fail with 400, got %d: %s", excessRec.Code, excessRec.Body.String())
	}
}

func seedAdminAndUsers(t *testing.T, database interface {
	CreateUserWithOptions(req *model.CreateUserRequest, passwordHash string) (*model.User, error)
}) (admin, userOne, userTwo *model.User) {
	t.Helper()

	var err error
	admin, err = database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "admin",
		Role:     "admin",
	}, "hash-admin")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	userOne, err = database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "user-one",
		Role:     "user",
	}, "hash-user-one")
	if err != nil {
		t.Fatalf("create user one: %v", err)
	}
	userTwo, err = database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "user-two",
		Role:     "user",
	}, "hash-user-two")
	if err != nil {
		t.Fatalf("create user two: %v", err)
	}
	return admin, userOne, userTwo
}

func injectCurrentUser(user *model.User) gin.HandlerFunc {
	return func(c *gin.Context) {
		setCurrentUser(c, user)
		c.Next()
	}
}

func setCurrentUser(c *gin.Context, user *model.User) {
	c.Set("user", user)
	c.Set("user_id", user.ID)
	c.Set("username", user.Username)
	c.Set("role", user.Role)
}

func collectAPIStringsFromNodes(nodes []model.Node) []string {
	result := make([]string, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node.Name)
	}
	return result
}

func collectAPIStringsFromUsers(users []model.User) []string {
	result := make([]string, 0, len(users))
	for _, user := range users {
		result = append(result, user.Username)
	}
	return result
}

func collectAPIStringsFromRules(rules []model.Rule) []string {
	result := make([]string, 0, len(rules))
	for _, rule := range rules {
		result = append(result, rule.Name)
	}
	return result
}

func sameAPIStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, item := range got {
		seen[item]++
	}
	for _, item := range want {
		seen[item]--
		if seen[item] < 0 {
			return false
		}
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

func boolPtr(value bool) *bool {
	return &value
}
