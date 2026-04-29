package unittest

// ─────────────────────────────────────────────────────────────────
// MANUAL MOCKS
//
// This is the simplest way to create a mock in Go.
// Just implement the interface and record the calls.
// ─────────────────────────────────────────────────────────────────

// ManualMockRepository — manual mock for the repository.
// 👉 Function fields let you change behavior in each test.
type ManualMockRepository struct {
	SaveFunc     func(order *Order) error
	FindByIDFunc func(id string) (*Order, error)

	// Record calls to assert on in the test
	SaveCalls     []*Order
	FindByIDCalls []string
}

func (m *ManualMockRepository) Save(order *Order) error {
	m.SaveCalls = append(m.SaveCalls, order)
	if m.SaveFunc != nil {
		return m.SaveFunc(order)
	}
	return nil // by default — do nothing
}

func (m *ManualMockRepository) FindByID(id string) (*Order, error) {
	m.FindByIDCalls = append(m.FindByIDCalls, id)
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(id)
	}
	return nil, ErrNotFound // by default — not found
}

// ManualMockPublisher — manual mock for the publisher.
type ManualMockPublisher struct {
	PublishFunc  func(eventType string, payload any) error
	PublishCalls []struct {
		EventType string
		Payload   any
	}
}

func (m *ManualMockPublisher) Publish(eventType string, payload any) error {
	m.PublishCalls = append(m.PublishCalls, struct {
		EventType string
		Payload   any
	}{eventType, payload})
	if m.PublishFunc != nil {
		return m.PublishFunc(eventType, payload)
	}
	return nil
}
