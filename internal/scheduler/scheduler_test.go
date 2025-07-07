package scheduler

import (
	"testing"

	"github.com/ubuygold/gogemini/internal/config"
	"github.com/ubuygold/gogemini/internal/db"
	"github.com/ubuygold/gogemini/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockKeyManager is a mock implementation of the Manager interface.
type MockKeyManager struct {
	mock.Mock
}

func (m *MockKeyManager) ReviveDisabledKeys() {
	m.Called()
}

func (m *MockKeyManager) CheckAllKeysHealth() {
	m.Called()
}

// MockDBService is a mock implementation of the db.Service interface.
type MockDBService struct {
	mock.Mock
}

func (m *MockDBService) ResetAllAPIKeyUsage() error {
	args := m.Called()
	return args.Error(0)
}

// Implement other methods of the db.Service interface, returning nil or zero values.
func (m *MockDBService) LoadActiveGeminiKeys() ([]model.GeminiKey, error) { return nil, nil }
func (m *MockDBService) ResetGeminiKeyFailureCount(key string) error      { return nil }
func (m *MockDBService) HandleGeminiKeyFailure(key string, threshold int) (bool, error) {
	return false, nil
}
func (m *MockDBService) CreateGeminiKey(key *model.GeminiKey) error { return nil }
func (m *MockDBService) BatchAddGeminiKeys(keys []string) error     { return nil }
func (m *MockDBService) BatchDeleteGeminiKeys(ids []uint) error     { return nil }
func (m *MockDBService) ListGeminiKeys(page, limit int, statusFilter string, minFailureCount int) ([]model.GeminiKey, int64, error) {
	return nil, 0, nil
}
func (m *MockDBService) GetGeminiKey(id uint) (*model.GeminiKey, error)    { return nil, nil }
func (m *MockDBService) UpdateGeminiKey(key *model.GeminiKey) error        { return nil }
func (m *MockDBService) DeleteGeminiKey(id uint) error                     { return nil }
func (m *MockDBService) IncrementGeminiKeyUsageCount(key string) error     { return nil }
func (m *MockDBService) UpdateGeminiKeyStatus(key, status string) error    { return nil }
func (m *MockDBService) CreateAPIKey(key *model.APIKey) error              { return nil }
func (m *MockDBService) ListAPIKeys() ([]model.APIKey, error)              { return nil, nil }
func (m *MockDBService) GetAPIKey(id uint) (*model.APIKey, error)          { return nil, nil }
func (m *MockDBService) UpdateAPIKey(key *model.APIKey) error              { return nil }
func (m *MockDBService) DeleteAPIKey(id uint) error                        { return nil }
func (m *MockDBService) IncrementAPIKeyUsageCount(key string) error        { return nil }
func (m *MockDBService) FindAPIKeyByKey(key string) (*model.APIKey, error) { return nil, nil }

func TestScheduler_ResetUsageJob(t *testing.T) {
	mockDB := new(MockDBService)
	mockKM := new(MockKeyManager)
	testConfig := &config.Config{}
	// We need to cast mockDB to db.Service because NewScheduler expects the interface, not the mock struct.
	var dbService db.Service = mockDB
	scheduler := NewScheduler(dbService, testConfig, mockKM)

	mockDB.On("ResetAllAPIKeyUsage").Return(nil).Once()

	scheduler.resetAPIKeyUsage()

	mockDB.AssertExpectations(t)
}

func TestScheduler_RunKeyRevivalJob(t *testing.T) {
	mockDB := new(MockDBService)
	mockKM := new(MockKeyManager)
	testConfig := &config.Config{}
	var dbService db.Service = mockDB
	scheduler := NewScheduler(dbService, testConfig, mockKM)

	mockKM.On("ReviveDisabledKeys").Return().Once()

	scheduler.runKeyRevivalJob()

	mockKM.AssertExpectations(t)
}

func TestScheduler_StartStop(t *testing.T) {
	mockDB := new(MockDBService)
	mockKM := new(MockKeyManager)
	testConfig := &config.Config{
		Scheduler: config.SchedulerConfig{
			KeyRevivalInterval: "@every 5m",
		},
	}
	var dbService db.Service = mockDB
	scheduler := NewScheduler(dbService, testConfig, mockKM)

	scheduler.Start()
	assert.NotNil(t, scheduler.c)
	entries := scheduler.c.Entries()
	assert.Len(t, entries, 3)

	scheduler.Stop()
	// After stopping, the context of the cron scheduler should be done.
	<-scheduler.c.Stop().Done()
}

func TestScheduler_RunDailyHealthCheckJob(t *testing.T) {
	mockDB := new(MockDBService)
	mockKM := new(MockKeyManager)
	testConfig := &config.Config{}
	var dbService db.Service = mockDB
	scheduler := NewScheduler(dbService, testConfig, mockKM)

	mockKM.On("CheckAllKeysHealth").Return().Once()

	scheduler.runDailyHealthCheckJob()

	mockKM.AssertExpectations(t)
}
