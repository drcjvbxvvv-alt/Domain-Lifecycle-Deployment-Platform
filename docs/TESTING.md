# TESTING.md — Testing Strategy & Patterns

> Claude Code: reference this when writing or modifying tests.

---

## Test File Organization

```
# Unit tests: same directory as source
internal/domain/service.go       → internal/domain/service_test.go
pkg/provider/dns/cloudflare.go   → pkg/provider/dns/cloudflare_test.go
api/handler/domain.go            → api/handler/domain_test.go
store/postgres/domain.go         → store/postgres/domain_test.go

# Integration tests: build-tagged
store/postgres/domain_integration_test.go  (//go:build integration)
pkg/provider/dns/cloudflare_integration_test.go (//go:build integration)

# Frontend tests
web/src/views/domains/__tests__/DomainList.spec.ts
web/src/stores/__tests__/domain.spec.ts
web/src/api/__tests__/domain.spec.ts
```

---

## Go Unit Tests

### Table-Driven Tests (mandatory pattern)

```go
func TestCanTransition(t *testing.T) {
    tests := []struct {
        name string
        from string
        to   string
        want bool
    }{
        {"inactive to deploying", "inactive", "deploying", true},
        {"inactive to active", "inactive", "active", false},
        {"active to degraded", "active", "degraded", true},
        {"blocked to retired", "blocked", "retired", true},
        {"retired to anything", "retired", "active", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := CanTransition(tt.from, tt.to)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Mocking Interfaces

```go
// Define mocks in the test file, NOT in a separate mock package.
// Keep mocks minimal — only implement methods under test.

type mockDomainStore struct {
    getFn    func(ctx context.Context, id int64) (*MainDomain, error)
    createFn func(ctx context.Context, d *MainDomain) error
    listFn   func(ctx context.Context, projectID int64, opts ListOpts) ([]MainDomain, error)
}

func (m *mockDomainStore) GetByID(ctx context.Context, id int64) (*MainDomain, error) {
    if m.getFn != nil {
        return m.getFn(ctx, id)
    }
    return nil, errors.New("not implemented")
}

// Usage in test
func TestDomainService_Deploy(t *testing.T) {
    store := &mockDomainStore{
        getFn: func(_ context.Context, id int64) (*MainDomain, error) {
            return &MainDomain{ID: id, Status: "inactive", Domain: "example.com"}, nil
        },
    }
    svc := NewService(store, nil, nil, nil, zap.NewNop())

    err := svc.Deploy(context.Background(), 1)
    assert.NoError(t, err)
}
```

### Testing HTTP Handlers

```go
func TestDomainHandler_List(t *testing.T) {
    gin.SetMode(gin.TestMode)

    svc := &mockDomainService{
        listFn: func(_ context.Context, projectID int64) ([]DomainResponse, error) {
            return []DomainResponse{
                {UUID: "uuid-1", Domain: "a.com", Status: "active"},
                {UUID: "uuid-2", Domain: "b.com", Status: "inactive"},
            }, nil
        },
    }
    handler := NewDomainHandler(svc, zap.NewNop())

    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = httptest.NewRequest("GET", "/api/v1/domains?project_id=1", nil)

    handler.List(c)

    assert.Equal(t, http.StatusOK, w.Code)

    var resp Response
    err := json.Unmarshal(w.Body.Bytes(), &resp)
    assert.NoError(t, err)
    assert.Equal(t, 0, resp.Code)
}
```

### Testing Template Rendering

```go
func TestRenderNginxConf(t *testing.T) {
    engine := NewTemplateEngine("../../templates")

    data := ConfData{
        MainDomain: "example.com",
        Subdomains: []SubdomainConf{
            {FQDN: "www.example.com", Upstream: "10.0.0.1:80"},
            {FQDN: "ws.example.com", Upstream: "10.0.0.2:8080"},
        },
    }

    result, err := engine.Render("website.conf.tmpl", data)
    assert.NoError(t, err)
    assert.Contains(t, result, "server_name www.example.com ws.example.com")
    assert.Contains(t, result, "proxy_pass http://10.0.0.1:80")

    // Validate generated conf is syntactically valid (if nginx available)
    if _, err := exec.LookPath("nginx"); err == nil {
        tmpFile := writeTempFile(t, result)
        cmd := exec.Command("nginx", "-t", "-c", tmpFile)
        assert.NoError(t, cmd.Run(), "generated nginx conf should be valid")
    }
}
```

---

## Go Integration Tests

Tag with `//go:build integration` — requires running PostgreSQL and Redis.

```go
//go:build integration

func TestDomainStore_Integration(t *testing.T) {
    // Use testcontainers or connect to Docker Compose services
    db := setupTestDB(t)
    t.Cleanup(func() { cleanupTestDB(db) })

    store := postgres.NewDomainStore(db)

    t.Run("create and retrieve", func(t *testing.T) {
        domain := &MainDomain{
            UUID:      "test-uuid-1",
            Domain:    "integration-test.com",
            ProjectID: 1,
            Status:    "inactive",
        }

        err := store.Create(context.Background(), domain)
        assert.NoError(t, err)
        assert.NotZero(t, domain.ID)

        retrieved, err := store.GetByID(context.Background(), domain.ID)
        assert.NoError(t, err)
        assert.Equal(t, "integration-test.com", retrieved.Domain)
    })
}
```

Run: `go test -tags=integration ./store/postgres/ -v`

---

## Frontend Tests (Vitest)

### Component Tests

```typescript
// web/src/views/domains/__tests__/DomainList.spec.ts
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import DomainList from '../DomainList.vue'
import { NDataTable, NTag } from 'naive-ui'

describe('DomainList', () => {
    it('renders domain table', async () => {
        const wrapper = mount(DomainList, {
            global: {
                plugins: [createTestingPinia({
                    initialState: {
                        domain: {
                            domains: [
                                { uuid: '1', domain: 'a.com', status: 'active' },
                                { uuid: '2', domain: 'b.com', status: 'blocked' },
                            ]
                        }
                    }
                })],
            },
            props: { projectId: 1 },
        })

        expect(wrapper.text()).toContain('a.com')
        expect(wrapper.text()).toContain('b.com')
    })
})
```

### Store Tests

```typescript
// web/src/stores/__tests__/domain.spec.ts
import { setActivePinia, createPinia } from 'pinia'
import { useDomainStore } from '../domain'
import { vi } from 'vitest'

vi.mock('@/api/domain', () => ({
    domainApi: {
        list: vi.fn().mockResolvedValue({
            data: { items: [{ uuid: '1', domain: 'test.com', status: 'active' }], total: 1 }
        })
    }
}))

describe('Domain Store', () => {
    beforeEach(() => setActivePinia(createPinia()))

    it('fetches domains', async () => {
        const store = useDomainStore()
        await store.fetchByProject(1)
        expect(store.domains).toHaveLength(1)
        expect(store.domains[0].domain).toBe('test.com')
    })
})
```

---

## Coverage Requirements

| Scope | Minimum Coverage | Notes |
|-------|-----------------|-------|
| internal/ (business logic) | 80% | Core value — must be well-tested |
| pkg/provider/ (adapters) | 60% | Interface compliance + error paths |
| api/handler/ | 70% | Request parsing + error responses |
| store/ | 50% (unit) | SQL correctness verified by integration |
| web/src/stores/ | 70% | State management logic |
| web/src/views/ | 50% | Basic render + interaction |

---

## Test Commands

```bash
# Run all unit tests
make test
# Equivalent to: go test ./... -count=1

# Run with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run integration tests (requires Docker services)
make test-integration
# Equivalent to: go test -tags=integration ./... -count=1 -v

# Run a specific test
go test ./internal/domain/ -run TestCanTransition -v

# Run frontend tests
cd web && npm run test
# Equivalent to: vitest run

# Run frontend tests with coverage
cd web && npm run test:coverage

# Lint
make lint
# Equivalent to: golangci-lint run ./... && cd web && npx eslint src/
```
