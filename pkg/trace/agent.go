package trace

//nolint:staticcheck
import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/sdk/resource"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/perfect-panel/server/pkg/logger"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

const (
	kindJaeger   = "jaeger"
	kindZipkin   = "zipkin"
	kindOtlpGrpc = "otlpgrpc"
	kindOtlpHttp = "otlphttp"
	kindFile     = "file"
	protocolUdp  = "udp"
)

var (
	agents = make(map[string]struct{})
	lock   sync.Mutex
	tp     *sdktrace.TracerProvider
)

// StartAgent starts an opentelemetry agent.
func StartAgent(c Config) {
	if c.Disabled {
		return
	}
	logger.Info("Starting agent")
	lock.Lock()
	defer lock.Unlock()

	_, ok := agents[c.Endpoint]
	if ok {
		return
	}

	// if error happens, let later calls run.
	if err := startAgent(c); err != nil {
		return
	}

	agents[c.Endpoint] = struct{}{}
}

// StopAgent shuts down the span processors in the order they were registered.
func StopAgent() {
	lock.Lock()
	defer lock.Unlock()

	if tp != nil {
		_ = tp.Shutdown(context.Background())
		tp = nil
	}
	clear(agents)
}

func createExporter(c Config) (sdktrace.SpanExporter, error) {
	// Just support jaeger and zipkin now, more for later
	switch c.Batcher {
	case kindJaeger:
		u, err := url.Parse(c.Endpoint)
		if err == nil && u.Scheme == protocolUdp {
			return jaeger.New(jaeger.WithAgentEndpoint(jaeger.WithAgentHost(u.Hostname()),
				jaeger.WithAgentPort(u.Port())))
		}
		return jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(c.Endpoint)))
	case kindZipkin:
		return zipkin.New(c.Endpoint)
	case kindOtlpGrpc:
		// Always treat trace exporter as optional component, so we use nonblock here,
		// otherwise this would slow down app start up even set a dial timeout here when
		// endpoint can not reach.
		// If the connection not dial success, the global otel ErrorHandler will catch error
		// when reporting data like other exporters.
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(c.Endpoint),
		}
		if len(c.OtlpHeaders) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(c.OtlpHeaders))
		}
		return otlptracegrpc.New(context.Background(), opts...)
	case kindOtlpHttp:
		// Not support flexible configuration now.
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(c.Endpoint),
		}

		if !c.OtlpHttpSecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if len(c.OtlpHeaders) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(c.OtlpHeaders))
		}
		if len(c.OtlpHttpPath) > 0 {
			opts = append(opts, otlptracehttp.WithURLPath(c.OtlpHttpPath))
		}
		return otlptracehttp.New(context.Background(), opts...)
	case kindFile:
		f, err := os.OpenFile(c.Endpoint, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return nil, fmt.Errorf("file exporter endpoint error: %s", err.Error())
		}
		if err := os.Chmod(c.Endpoint, 0o600); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("secure file exporter endpoint: %s", err.Error())
		}
		return stdouttrace.New(stdouttrace.WithWriter(f))
	default:
		return nil, fmt.Errorf("unknown exporter: %s", c.Batcher)
	}
}

func startAgent(c Config) error {
	AddResources(semconv.ServiceNameKey.String(c.Name))
	sampler := c.Sampler
	// Without an exporter there is no trace destination. Keep propagation and
	// request IDs, but avoid recording every span in memory.
	if strings.TrimSpace(c.Endpoint) == "" {
		sampler = 0
	}
	if sampler < 0 {
		sampler = 0
	} else if sampler > 1 {
		sampler = 1
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampler))),
		// Record information about this application in a Resource.
		sdktrace.WithResource(resource.NewSchemaless(attrResources...)),
	}

	if len(c.Endpoint) > 0 {
		exp, err := createExporter(c)
		if err != nil {
			logger.Error(err)
			return err
		}

		// Always be sure to batch in production.
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	tp = sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Errorf("[otel] error: %v", err)
	}))

	return nil
}
