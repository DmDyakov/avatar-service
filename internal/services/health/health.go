// Package health реализует сервис проверки состояния приложения.
package health

import "context"

// Pinger — компонент, который можно проверить.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Service агрегирует проверки компонентов.
type Service struct {
	components map[string]Pinger
}

func New() *Service {
	return &Service{
		components: make(map[string]Pinger),
	}
}

// Register регистрирует компонент.
func (s *Service) Register(name string, p Pinger) {
	s.components[name] = p
}

// Status — результат проверки.
type Status struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components"`
}

// Check проверяет все компоненты.
func (s *Service) Check(ctx context.Context) (*Status, int) {
	components := make(map[string]string, len(s.components))
	healthy := true

	for name, p := range s.components {
		if err := p.Ping(ctx); err != nil {
			components[name] = "error: " + err.Error()
			healthy = false
		} else {
			components[name] = "ok"
		}
	}

	status := "ok"
	code := 200
	if !healthy {
		status = "degraded"
		code = 503
	}

	return &Status{
		Status:     status,
		Components: components,
	}, code
}
