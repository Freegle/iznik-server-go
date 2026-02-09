# V2 API Coding Guide

This guide defines patterns for all Go API handlers in iznik-server-go. Follow these patterns exactly when implementing new endpoints.

## Handler Structure (Read)

Every read handler follows this sequence:

```go
func GetThing(c *fiber.Ctx) error {
    // 1. AUTH
    myid := user.WhoAmI(c)

    // 2. PARSE parameters
    id, err := strconv.ParseUint(c.Params("id"), 10, 64)
    if err != nil {
        return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
    }

    // 3. DB connection
    db := database.DBConn

    // 4. FETCH with goroutines for independent queries
    var wg sync.WaitGroup
    var thing Thing
    var related []Related

    wg.Add(2)
    go func() {
        defer wg.Done()
        db.Raw("SELECT ... FROM things WHERE id = ?", id).Scan(&thing)
    }()
    go func() {
        defer wg.Done()
        db.Raw("SELECT ... FROM related WHERE thing_id = ?", id).Scan(&related)
    }()
    wg.Wait()

    // 5. PRIVACY filtering
    if thing.Userid != myid {
        thing.Textbody = hideEmails(thing.Textbody)
    }

    // 6. ASSEMBLE response
    thing.Related = related

    // 7. RESPOND
    return c.JSON(thing)
}
```

## Handler Structure (Write)

Write handlers follow a similar but distinct pattern:

```go
func CreateThing(c *fiber.Ctx) error {
    // 1. AUTH - require login
    myid := user.WhoAmI(c)
    if myid == 0 {
        return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
    }

    // 2. PARSE body
    var req CreateRequest
    if err := c.BodyParser(&req); err != nil {
        return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
    }

    // 3. VALIDATE required fields
    if req.Title == "" {
        return fiber.NewError(fiber.StatusBadRequest, "title is required")
    }

    // 4. AUTHZ - check permissions if needed
    if !canModify(myid, req.ID) {
        return fiber.NewError(fiber.StatusForbidden, "Not authorized")
    }

    // 5. DB write
    db := database.DBConn
    result := db.Exec("INSERT INTO things (...) VALUES (?)", ...)
    if result.Error != nil {
        return fiber.NewError(fiber.StatusInternalServerError, "Failed to create")
    }

    // 6. GET ID of inserted row
    var id uint64
    db.Raw("SELECT id FROM things WHERE userid = ? ORDER BY id DESC LIMIT 1", myid).Scan(&id)

    // 7. SIDE EFFECTS - queue async tasks
    queue.QueueTask(db, queue.TaskPushNotifyGroupMods, map[string]interface{}{
        "group_id": req.GroupID,
    })

    // 8. RESPOND
    return c.JSON(fiber.Map{"id": id})
}
```

## Authentication

Always use `user.WhoAmI(c)` for authentication. Returns `uint64` user ID or `0` if not logged in.

```go
myid := user.WhoAmI(c)

// For endpoints requiring login:
if myid == 0 {
    return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
}
```

Read endpoints generally allow anonymous access (myid == 0) with reduced data. Write endpoints require login.

## Parameter Parsing

### Path parameters
```go
id, err := strconv.ParseUint(c.Params("id"), 10, 64)
if err != nil {
    return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
}
```

### Comma-separated IDs
```go
ids := strings.Split(c.Params("ids"), ",")
```

### Query parameters
```go
limit := c.QueryInt("limit", 20)            // with default
groupid := c.QueryInt("groupid", 0)
context := c.Query("context")               // string
```

### Request body (POST/PATCH)
```go
type CreateRequest struct {
    Title   string `json:"title"`
    GroupID uint64 `json:"groupid"`
}
var req CreateRequest
if err := c.BodyParser(&req); err != nil {
    return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
}
```

### Optional PATCH fields (use pointers)
```go
type PatchRequest struct {
    Title       *string `json:"title,omitempty"`
    Description *string `json:"description,omitempty"`
    Pending     *int    `json:"pending,omitempty"`
}
// Then update only non-nil fields:
if req.Title != nil {
    db.Exec("UPDATE things SET title = ? WHERE id = ?", *req.Title, id)
}
```

## Database Queries

### Use raw SQL
Prefer `db.Raw()` for reads and `db.Exec()` for writes. Avoid GORM query builder.

```go
db := database.DBConn

// Read - scan into struct
var thing Thing
db.Raw("SELECT id, title, userid FROM things WHERE id = ?", id).Scan(&thing)

// Read - scan into slice
var things []Thing
db.Raw("SELECT id, title FROM things WHERE groupid = ?", gid).Scan(&things)

// Write
result := db.Exec("INSERT INTO things (userid, title) VALUES (?, ?)", myid, title)
if result.Error != nil {
    return fiber.NewError(fiber.StatusInternalServerError, "Database error")
}

// Get last insert ID
var id uint64
db.Raw("SELECT id FROM things WHERE userid = ? ORDER BY id DESC LIMIT 1", myid).Scan(&id)
```

### Goroutines for parallel queries
Use `sync.WaitGroup` for 2+ independent queries. One goroutine per independent query.

```go
var wg sync.WaitGroup
var thing Thing
var groups []uint64
var image *Image

wg.Add(3)
go func() {
    defer wg.Done()
    db.Raw("SELECT ... FROM things WHERE id = ?", id).Scan(&thing)
}()
go func() {
    defer wg.Done()
    db.Raw("SELECT groupid FROM thing_groups WHERE thingid = ?", id).Scan(&groups)
}()
go func() {
    defer wg.Done()
    db.Raw("SELECT ... FROM thing_images WHERE thingid = ? LIMIT 1", id).Scan(&image)
}()
wg.Wait()
```

Use `sync.Mutex` only when goroutines write to shared slices/maps:
```go
var mu sync.Mutex
var results []Thing

go func() {
    defer wg.Done()
    var batch []Thing
    db.Raw("...").Scan(&batch)
    mu.Lock()
    results = append(results, batch...)
    mu.Unlock()
}()
```

### Spatial queries
Use the `utils.SRID` constant for spatial operations:
```go
db.Raw(fmt.Sprintf("SELECT ... ST_Distance(geometry, ST_GeomFromText('POINT(%f %f)', %d)) ...",
    lng, lat, utils.SRID))
```

## Response Formatting

### Single item
```go
return c.JSON(thing)
```

### Single item vs array (ID-based lookup)
```go
if len(ids) == 1 {
    if len(results) == 1 {
        return c.JSON(results[0])
    }
    return fiber.NewError(fiber.StatusNotFound, "Not found")
}
return c.JSON(results)
```

### Empty arrays - MUST return `[]` not `null`
```go
if len(results) == 0 {
    return c.JSON(make([]string, 0))  // Forces JSON []
}
return c.JSON(results)
```

### Write response
```go
return c.JSON(fiber.Map{"id": id})
return c.JSON(fiber.Map{"ret": 0})
```

### Error responses
```go
return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
return fiber.NewError(fiber.StatusForbidden, "Not authorized")
return fiber.NewError(fiber.StatusNotFound, "Not found")
return fiber.NewError(fiber.StatusInternalServerError, "Database error")
```

## Privacy Filtering

Always check ownership before returning sensitive data:

```go
if thing.Userid != myid {
    thing.Textbody = hideEmails(thing.Textbody)
    thing.Textbody = hidePhones(thing.Textbody)
    thing.Email = ""
}
```

The `misc` package has helpers for email/phone hiding. Check existing handlers like `message.go` for the exact patterns used.

## Authorization Patterns

### Owner check
```go
func canModify(myid uint64, thingID uint64) bool {
    db := database.DBConn
    var ownerID uint64
    db.Raw("SELECT userid FROM things WHERE id = ?", thingID).Scan(&ownerID)
    if ownerID == myid {
        return true
    }
    // Also check if user is a moderator of the associated group
    return isModerator(myid, thingID)
}
```

### Moderator check
```go
func isModerator(myid uint64, thingID uint64) bool {
    db := database.DBConn
    var count int64
    db.Raw("SELECT COUNT(*) FROM memberships m "+
        "INNER JOIN thing_groups tg ON tg.groupid = m.groupid "+
        "WHERE m.userid = ? AND tg.thingid = ? AND m.role IN ('Owner', 'Moderator')",
        myid, thingID).Scan(&count)
    return count > 0
}
```

### Admin/Support check
```go
// Use user package utilities for admin checks
if user.IsAdmin(myid) { ... }
```

## Side Effects (Background Tasks)

For async operations (emails, push notifications), use the background task queue:

```go
import "github.com/freegle/iznik-server-go/queue"

// Queue a push notification
queue.QueueTask(db, queue.TaskPushNotifyGroupMods, map[string]interface{}{
    "group_id": groupID,
})

// Queue an email
queue.QueueTask(db, queue.TaskEmailChitchatReport, map[string]interface{}{
    "user_id":    myid,
    "user_name":  userName,
    "user_email": userEmail,
    "newsfeed_id": feedID,
    "reason":     reason,
})
```

Task types are defined as constants in `queue/queue.go`. Add new constants there when implementing new email types.

## Struct Design

### JSON tags
```go
type Thing struct {
    ID       uint64    `json:"id" gorm:"primary_key"`
    Userid   uint64    `json:"userid"`
    Title    string    `json:"title"`
    Internal string    `json:"-"`              // Never serialized
    Groups   []uint64  `json:"groups" gorm:"-"` // Computed, not in DB
    Image    *Image    `json:"image" gorm:"-"`  // Fetched separately
}
```

### TableName override
```go
func (Thing) TableName() string {
    return "things"  // Helps avoid GORM race conditions in testing
}
```

## Route Registration

Add routes in `router/routes.go`, always registering both `/api` and `/apiv2` paths:

```go
for _, rg := range []fiber.Router{api, apiv2} {
    // Thing endpoints
    rg.Get("/thing", thing.List)
    rg.Get("/thing/:id", thing.Single)
    rg.Post("/thing", thing.Create)
    rg.Patch("/thing", thing.Update)
    rg.Delete("/thing/:id", thing.Delete)
}
```

Add Swagger annotations above each route:
```go
// @Router /thing/:id [get]
// @Summary Get thing by ID
// @Tags Thing
// @Param id path int true "Thing ID" example(12345)
// @Success 200 {object} thing.Thing
rg.Get("/thing/:id", thing.Single)
```

After adding routes, run `./generate-swagger.sh`.

## Testing

Tests live in `test/` directory, one file per domain (e.g., `test/volunteering_test.go`).

### Test structure
```go
func TestCreateThing(t *testing.T) {
    // ARRANGE - create test data using factory functions
    prefix := uniquePrefix("CreateThing")
    groupID := CreateTestGroup(t, prefix)
    userID := CreateTestUser(t, prefix, "Member")
    CreateTestMembership(t, userID, groupID, "Member")
    _, token := CreateTestSession(t, userID)

    // ACT - make HTTP request
    body := fmt.Sprintf(`{"title":"Test","groupid":%d}`, groupID)
    req := httptest.NewRequest("POST", "/api/thing", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+token)
    resp, _ := getApp().Test(req)

    // ASSERT
    assert.Equal(t, fiber.StatusOK, resp.StatusCode)
    respBody := rsp(resp)
    var result map[string]interface{}
    json.Unmarshal(respBody, &result)
    assert.NotZero(t, result["id"])
}
```

### Factory functions (in testUtils.go)
```go
CreateTestGroup(t, prefix)                        // Returns groupID
CreateTestUser(t, prefix, "Member")               // Returns userID
CreateTestMembership(t, userID, groupID, "Member") // Returns membershipID
CreateTestSession(t, userID)                       // Returns (sessionID, token)
CreatePersistentToken(t, userID, sessionID)        // Returns auth2 token string
```

### Test patterns
- Use `uniquePrefix(testName)` for unique test data
- Always clean up with real DB rows (FK constraints are enforced)
- Use `getApp().Test(req)` to make requests against the Fiber app
- Use `rsp(resp)` to read response body as `[]byte`
- Test both authenticated and unauthenticated access
- Test 404 for missing resources, 403 for unauthorized modifications

### Unauthenticated request
```go
req := httptest.NewRequest("GET", "/api/thing/1", nil)
resp, _ := getApp().Test(req)
assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
```

### Authenticated request
```go
req := httptest.NewRequest("GET", "/api/thing/"+id, nil)
req.Header.Set("Authorization", "Bearer "+token)
resp, _ := getApp().Test(req)
```

## Common Pitfalls

1. **Nil slices serialize as `null`** - Always use `make([]T, 0)` for empty collections.
2. **GORM race conditions** - Define `TableName()` on all structs.
3. **User location** - Users don't have lat/lng columns. Location is via `lastlocation` FK to `locations` table, or `settings` JSON field (`mylocation`).
4. **Foreign keys enforced** - Tests must create real rows for FK references (groups, users, etc.).
5. **Don't store `*fiber.Ctx`** - Never pass the context to goroutines or store it.
6. **HTML escaping** - Use `html.EscapeString()` for user-generated content in responses.
7. **LAST_INSERT_ID()** - Don't rely on it with GORM. Query `ORDER BY id DESC LIMIT 1` instead.
