package conf

import "testing"

type Server struct {
	Host string `yaml:"Host" default:"localhost"`
	Port int    `yaml:"Port" default:"8080"`
}

type Config struct {
	Server Server `yaml:"Server"`
}

type floatingConfig struct {
	Sampler float64 `default:"1.0"`
}

func TestConfigLoad(t *testing.T) {
	var c Config
	MustLoad("./config_test.yaml", &c)
	t.Logf("config: %+v", c)
}

func TestDefaultsSupportFloat64(t *testing.T) {
	var c floatingConfig
	setDefaults(&c)
	if c.Sampler != 1.0 {
		t.Fatalf("Sampler = %v, want 1.0", c.Sampler)
	}
}
