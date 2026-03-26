package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/flowgate/flowgate/internal/panel/hub"
	"github.com/flowgate/flowgate/internal/panel/model"
)

func TestExpiredUserLoginRejected(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	expiredAt := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	if _, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username:  "expired-user",
		Role:      "user",
		ExpiresAt: &expiredAt,
	}, string(passwordHash)); err != nil {
		t.Fatalf("create expired user: %v", err)
	}

	handler := &AuthHandler{DB: database, JWTSecret: "test-secret"}
	body, _ := json.Marshal(map[string]any{
		"username": "expired-user",
		"password": "secret123",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req

	handler.Login(ctx)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected expired login status 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResellerScopeForUsers(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	_, reseller, _, _ := seedPhase3Actors(t, database)
	userHandler := &UserHandler{DB: database}

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	setCurrentUser(ctx, reseller)

	userHandler.ListUsers(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("list users status = %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Users []model.User `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode users response: %v", err)
	}
	if got, want := collectAPIStringsFromUsers(resp.Users), []string{"reseller", "child"}; !sameAPIStrings(got, want) {
		t.Fatalf("unexpected visible users: got %v want %v", got, want)
	}
}

func TestResellerScopeForNodesAndRules(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	_, reseller, child, outsider := seedPhase3Actors(t, database)

	resellerNode, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "reseller-node"}, reseller.ID)
	if err != nil {
		t.Fatalf("create reseller node: %v", err)
	}
	childNode, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "child-node"}, child.ID)
	if err != nil {
		t.Fatalf("create child node: %v", err)
	}
	outsiderNode, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "outsider-node"}, outsider.ID)
	if err != nil {
		t.Fatalf("create outsider node: %v", err)
	}

	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     resellerNode.ID,
		Name:       "reseller-rule",
		Protocol:   "tcp",
		ListenPort: 33001,
		TargetAddr: "127.0.0.1",
		TargetPort: 8080,
	}, reseller.ID); err != nil {
		t.Fatalf("create reseller rule: %v", err)
	}
	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     childNode.ID,
		Name:       "child-rule",
		Protocol:   "tcp",
		ListenPort: 33002,
		TargetAddr: "127.0.0.1",
		TargetPort: 8081,
	}, child.ID); err != nil {
		t.Fatalf("create child rule: %v", err)
	}
	outsiderRule, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     outsiderNode.ID,
		Name:       "outsider-rule",
		Protocol:   "tcp",
		ListenPort: 33003,
		TargetAddr: "127.0.0.1",
		TargetPort: 8082,
	}, outsider.ID)
	if err != nil {
		t.Fatalf("create outsider rule: %v", err)
	}

	nodeHandler := &NodeHandler{DB: database}
	ruleHandler := &RuleHandler{DB: database}

	listNodesReq := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	listNodesRec := httptest.NewRecorder()
	listNodesCtx, _ := gin.CreateTestContext(listNodesRec)
	listNodesCtx.Request = listNodesReq
	setCurrentUser(listNodesCtx, reseller)
	nodeHandler.ListNodes(listNodesCtx)
	if listNodesRec.Code != http.StatusOK {
		t.Fatalf("list nodes status = %d: %s", listNodesRec.Code, listNodesRec.Body.String())
	}

	var nodesResp struct {
		Nodes []model.Node `json:"nodes"`
	}
	if err := json.Unmarshal(listNodesRec.Body.Bytes(), &nodesResp); err != nil {
		t.Fatalf("decode nodes response: %v", err)
	}
	if got, want := collectAPIStringsFromNodes(nodesResp.Nodes), []string{"reseller-node", "child-node"}; !sameAPIStrings(got, want) {
		t.Fatalf("unexpected visible nodes: got %v want %v", got, want)
	}

	listRulesReq := httptest.NewRequest(http.MethodGet, "/api/rules", nil)
	listRulesRec := httptest.NewRecorder()
	listRulesCtx, _ := gin.CreateTestContext(listRulesRec)
	listRulesCtx.Request = listRulesReq
	setCurrentUser(listRulesCtx, reseller)
	ruleHandler.ListRules(listRulesCtx)
	if listRulesRec.Code != http.StatusOK {
		t.Fatalf("list rules status = %d: %s", listRulesRec.Code, listRulesRec.Body.String())
	}

	var rulesResp struct {
		Rules []model.Rule `json:"rules"`
	}
	if err := json.Unmarshal(listRulesRec.Body.Bytes(), &rulesResp); err != nil {
		t.Fatalf("decode rules response: %v", err)
	}
	if got, want := collectAPIStringsFromRules(rulesResp.Rules), []string{"reseller-rule", "child-rule"}; !sameAPIStrings(got, want) {
		t.Fatalf("unexpected visible rules: got %v want %v", got, want)
	}

	outsiderRuleReq := httptest.NewRequest(http.MethodGet, "/api/rules/outsider", nil)
	outsiderRuleRec := httptest.NewRecorder()
	outsiderRuleCtx, _ := gin.CreateTestContext(outsiderRuleRec)
	outsiderRuleCtx.Request = outsiderRuleReq
	outsiderRuleCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(outsiderRule.ID, 10)}}
	setCurrentUser(outsiderRuleCtx, reseller)
	ruleHandler.GetRule(outsiderRuleCtx)
	if outsiderRuleRec.Code != http.StatusNotFound {
		t.Fatalf("expected reseller outsider rule lookup to return 404, got %d: %s", outsiderRuleRec.Code, outsiderRuleRec.Body.String())
	}
}

func TestUserCannotAccessManagerRoute(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	_, _, child, _ := seedPhase3Actors(t, database)
	nodeHandler := &NodeHandler{DB: database}

	router := gin.New()
	router.POST("/api/nodes",
		injectCurrentUser(child),
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

func TestManagerCreateRuleRespectsMaxRules(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	reseller, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "limited-reseller",
		Role:     "reseller",
		MaxRules: 1,
	}, "hash-reseller")
	if err != nil {
		t.Fatalf("create reseller: %v", err)
	}

	node, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "limited-node"}, reseller.ID)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     node.ID,
		Name:       "existing-rule",
		Protocol:   "tcp",
		ListenPort: 34001,
		TargetAddr: "127.0.0.1",
		TargetPort: 8080,
		SpeedLimit: 64,
	}, reseller.ID); err != nil {
		t.Fatalf("create existing rule: %v", err)
	}

	ruleHandler := &RuleHandler{DB: database, Hub: hub.New(database)}
	body, _ := json.Marshal(map[string]any{
		"node_id":     node.ID,
		"name":        "blocked-rule",
		"protocol":    "tcp",
		"listen_port": 34002,
		"target_addr": "127.0.0.1",
		"target_port": 8081,
		"speed_limit": 64,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	setCurrentUser(ctx, reseller)

	ruleHandler.CreateRule(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected max-rules rejection to return 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestManagerCreateAndUpdateRuleRejectExcessBandwidth(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	database := newTestAPIContextDatabase(t)
	reseller, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username:       "bandwidth-reseller",
		Role:           "reseller",
		BandwidthLimit: 128,
	}, "hash-reseller")
	if err != nil {
		t.Fatalf("create reseller: %v", err)
	}

	node, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "bandwidth-node"}, reseller.ID)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	ruleHandler := &RuleHandler{DB: database, Hub: hub.New(database)}
	createBody, _ := json.Marshal(map[string]any{
		"node_id":     node.ID,
		"name":        "too-fast-rule",
		"protocol":    "tcp",
		"listen_port": 35001,
		"target_addr": "127.0.0.1",
		"target_port": 8080,
		"speed_limit": 256,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/rules", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRec)
	createCtx.Request = createReq
	setCurrentUser(createCtx, reseller)

	ruleHandler.CreateRule(createCtx)

	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("expected create bandwidth rejection to return 400, got %d: %s", createRec.Code, createRec.Body.String())
	}

	rule, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     node.ID,
		Name:       "normal-rule",
		Protocol:   "tcp",
		ListenPort: 35002,
		TargetAddr: "127.0.0.1",
		TargetPort: 8081,
		SpeedLimit: 64,
	}, reseller.ID)
	if err != nil {
		t.Fatalf("create baseline rule: %v", err)
	}

	updateBody, _ := json.Marshal(map[string]any{
		"speed_limit": 256,
	})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/rules/"+strconv.FormatInt(rule.ID, 10), bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	updateCtx, _ := gin.CreateTestContext(updateRec)
	updateCtx.Request = updateReq
	updateCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(rule.ID, 10)}}
	setCurrentUser(updateCtx, reseller)

	ruleHandler.UpdateRule(updateCtx)

	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected update bandwidth rejection to return 400, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
}

func seedPhase3Actors(t *testing.T, database interface {
	CreateUserWithOptions(req *model.CreateUserRequest, passwordHash string) (*model.User, error)
}) (admin, reseller, child, outsider *model.User) {
	t.Helper()

	var err error
	admin, err = database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "admin",
		Role:     "admin",
	}, "hash-admin")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	reseller, err = database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "reseller",
		Role:     "reseller",
		ParentID: &admin.ID,
	}, "hash-reseller")
	if err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	child, err = database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "child",
		Role:     "user",
		ParentID: &reseller.ID,
	}, "hash-child")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	outsider, err = database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "outsider",
		Role:     "user",
	}, "hash-outsider")
	if err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	return admin, reseller, child, outsider
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
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
