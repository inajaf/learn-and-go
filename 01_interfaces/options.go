package interfaces

import "log/slog"

// =============================================================================
//Functional Options - a pattern for flexible configuration
// =============================================================================
//
//Problem: Constructor with many parameters
//
//   NewOrderService(repo, logger, publisher, timeout, maxRetries)  // 😱
//
//Solution: Functional Options - each option is a function
//
//   NewOrderService(repo,
//WithLogger(logger), // optional
//WithEventPublisher(pub), // optional
//   )
//
//Advantages:
//- Mandatory dependencies - in the constructor parameters
//- Optional - through Option functions
//- Default values ​​- if the option is not specified
//- Easily add a new option without changing the signature
//- Self-documenting: WithLogger(l) is clearer than a positional parameter
//
//🏭 In production: this pattern is used by grpc.NewServer(), http.NewServeMux(),
//zap.New(), slog.New() and almost all serious Go libraries.

//ServiceOption is a function that configures OrderService.
//👉 This is the key type of pattern: option is a function that modifies the config.
type ServiceOption func(*serviceConfig)

//serviceConfig - internal configuration structure.
//👉 We do not export - the user should not know the internal structure.
//
//Access only through With functions.
type serviceConfig struct {
	logger    *slog.Logger
	publisher EventPublisher
}

//defaultConfig returns the default configuration.
func defaultConfig() serviceConfig {
	return serviceConfig{
		logger: slog.Default(), //standard slog logger
	}
}

//WithLogger installs the logger.
//
//	svc := NewOrderService(repo, WithLogger(myLogger))
func WithLogger(logger *slog.Logger) ServiceOption {
	return func(c *serviceConfig) {
		c.logger = logger
	}
}

//WithEventPublisher sets the publisher for publishing events.
//
//	svc := NewOrderService(repo, WithEventPublisher(kafkaPublisher))
func WithEventPublisher(pub EventPublisher) ServiceOption {
	return func(c *serviceConfig) {
		c.publisher = pub
	}
}

//NewOrderService - service constructor with Functional Options.
//
//👉 repo is a required dependency (without it there is no point in the service).
//
//opts - optional settings, each applied to the config.
//
//Usage example:
//
//// Minimum configuration - only required:
//	svc := NewOrderService(repo)
//
//// Full configuration - with options:
//	svc := NewOrderService(repo,
//	    WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil))),
//	    WithEventPublisher(kafkaPub),
//	)
func NewOrderService(repo OrderRepository, opts ...ServiceOption) *OrderService {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &OrderService{
		repo:   repo,
		logger: cfg.logger,
		pub:    cfg.publisher,
	}
}
